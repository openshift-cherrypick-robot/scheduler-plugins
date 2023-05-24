/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

/*
 * This is needed because of https://github.com/kubernetes/kubernetes/issues/110660
 * Fixed in kubernetes 1.25.0, but enhancements are not backported.
 * So we need to wait until the scheduler plugins repo consumes k8s >= 1.25.0, then
 * We can just add a client-go/cache Indexer and get rid of this workaround:
 *
 * func newNodeNameIndexer(handle framework.Handle) (IndexGetter, error) {
 *     podInformer := handle.SharedInformerFactory().Core().V1().Pods()
 *     err = podInformer.Informer().GetIndexer().AddIndexers(cache.Indexers{
 *         NodeNameIndexer: indexByPodNodeName,
 *     })
 *     if err != nil {
 *     		return nil, err
 *     }
 *     return podInformer.Informer().GetIndexer(), nil
 * }
 * (possibly inlined in the calling site)
 */

// TODO: tests using multiple goroutines (+ race detector)

type podObservedState int

const (
	podObservedReserved podObservedState = 1 << iota
	podObservedBound
)

func (pos podObservedState) String() string {
	if pos == podObservedReserved {
		return "reserved"
	}
	return "bound"
}

type NodeIndexer interface {
	GetPodNamespacedNamesByNode(logID, nodeName string) ([]types.NamespacedName, error)
	TrackReservedPod(pod *corev1.Pod, nodeName string)
	UntrackReservedPod(pod *corev1.Pod, nodeName string)
}

type nodeNameIndexer struct {
	rwLock sync.RWMutex
	// nodeName -> podUID -> podNamespacedName
	nodeToPodsMap map[string]map[types.UID]podData
	// nodeName -> podUID
	// we need this map to handle possible deletions when pods have been are reserved but not yet detected running
	// note we do NOT clean up this map when pods are detected running, but only on pod deletion - or on Unreserve.
	podUIDToCandidateNodeMap map[types.UID]string
}

type podData struct {
	namespacedName types.NamespacedName
	observedState  podObservedState
}

func NewNodeNameIndexer(podInformer k8scache.SharedInformer) NodeIndexer {
	nni := &nodeNameIndexer{
		nodeToPodsMap:            make(map[string]map[types.UID]podData),
		podUIDToCandidateNodeMap: make(map[types.UID]string),
	}
	podInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				klog.V(3).InfoS("nrtcache: nni: add unsupported object %T", obj)
				return
			}
			nni.addPod(pod)
		},
		// need to track pods scheduled by the default scheduler
		UpdateFunc: func(oldObj, newObj interface{}) {
			pod, ok := newObj.(*corev1.Pod)
			if !ok {
				klog.V(3).InfoS("nrtcache: nni: update unsupported object %T", newObj)
				return
			}
			nni.addPod(pod)
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				klog.V(3).InfoS("nrtcache: nni: delete unsupported object %T", obj)
				return
			}
			nni.deletePod(pod)
		},
	})
	return nni
}

func (nni *nodeNameIndexer) GetPodNamespacedNamesByNode(logID, nodeName string) ([]types.NamespacedName, error) {
	nni.rwLock.RLock()
	defer nni.rwLock.RUnlock()
	data, ok := nni.nodeToPodsMap[nodeName]
	if !ok {
		klog.V(6).InfoS("nrtcache: nni GET ", "logID", logID, "node", nodeName, "pods", "NONE")
		return []types.NamespacedName{}, nil
	}

	objs := make([]types.NamespacedName, 0, len(data))
	for _, item := range data {
		objs = append(objs, item.namespacedName)
	}
	klog.V(6).InfoS("nrtcache: nni GET ", "logID", logID, "node", nodeName, "pods", namespacedNameListToString(objs))
	return objs, nil
}

func (nni *nodeNameIndexer) TrackReservedPod(pod *corev1.Pod, nodeName string) {
	nni.rwLock.Lock()
	defer nni.rwLock.Unlock()
	nni.podUIDToCandidateNodeMap[pod.UID] = nodeName
	nni.insertPod(pod, nodeName, podObservedReserved)
	klog.V(6).InfoS("nrtcache: nni RES ", "logID", klog.KObj(pod), "node", nodeName, "podUID", pod.UID)
}

func (nni *nodeNameIndexer) UntrackReservedPod(pod *corev1.Pod, nodeName string) {
	nni.rwLock.Lock()
	defer nni.rwLock.Unlock()
	delete(nni.podUIDToCandidateNodeMap, pod.UID)
	obsState, ok := nni.getPodObservedState(pod, nodeName)
	if ok && obsState == podObservedBound {
		delete(nni.nodeToPodsMap[nodeName], pod.UID)
	}
	klog.V(6).InfoS("nrtcache: nni UNR ", "logID", klog.KObj(pod), "node", nodeName, "podUID", pod.UID, "obsState", obsState)
}

func (nni *nodeNameIndexer) addPod(pod *corev1.Pod) {
	if pod.Spec.NodeName == "" {
		klog.V(4).InfoS("nrtcache: nni WARN", "logID", klog.KObj(pod), "node", "EMPTY", "podUID", pod.UID, "phase", pod.Status.Phase)
		return
	}
	if pod.Status.Phase != corev1.PodRunning {
		// We should only consider pod which are consuming resources. But it's also paramount to take into account what the
		// node side can do. The primary source of data for node agents is the podresources API, and the podresources API
		// cannot do any filtering based on pod runtime status. Accessing this data from other sources (e.g. apiserver, runtime)
		// is either hard (runtime) or should be avoided for performance reasons (apiserver). So the only compromise is
		// to overshoot and - incorrectly - account for ALL the pods reported on the node. Let's at least acknowledge
		// this fact and be pretty loud when it occurs.
		klog.V(4).InfoS("nrtcache: nni WARN", "logID", klog.KObj(pod), "node", pod.Spec.NodeName, "podUID", pod.UID, "phase", pod.Status.Phase)
		// intentional fallback
	}
	nni.rwLock.Lock()
	defer nni.rwLock.Unlock()
	klog.V(6).InfoS("nrtcache: nni ADD ", "logID", klog.KObj(pod), "node", pod.Spec.NodeName, "podUID", pod.UID)
	nni.insertPod(pod, pod.Spec.NodeName, podObservedBound)
}

func (nni *nodeNameIndexer) deletePod(pod *corev1.Pod) {
	nni.rwLock.Lock()
	defer nni.rwLock.Unlock()

	nodeName := pod.Spec.NodeName
	wasReserved := false
	if nodeName == "" {
		// we're deleting a pod which passed the reserve stage but never got to the binding stage.
		// Should be unlikely, but can happen!
		nodeName, wasReserved = nni.podUIDToCandidateNodeMap[pod.UID]
	}
	if nodeName == "" { // how come?
		klog.V(6).InfoS("nrtcache: nni DEL ignored - missing node!", "logID", klog.KObj(pod), "podUID", pod.UID)
		return
	}

	// so maps in golang (up to 1.19 included) do NOT release the memory for the buckets even on delete
	// of their elements. We use maps -even nested- heavily, so are we at risk of leaking memory and
	// being eventually OOM-killed?
	// *YES*. But we can manage.
	// First of all, the amount of nodes in a cluster is expected to be stable.
	// OTOH, we can expect pod churn and a non-negligible amount of intermixed add/delete. This means the inner
	// maps can leak. We can't estimate how frequent or how bad this can be yet.
	// if this becomes a real bug, the fix is to recreate the inner maps with only the live keys, dropping
	// the old leaky map. We should probably do this here, on delete, but not every time but once the pod
	// churn (easily trackable with an integer counting the deletes) crosses a threshold, whose value is TBD.
	//
	// for more details: https://github.com/golang/go/issues/20135
	delete(nni.nodeToPodsMap[nodeName], pod.UID)
	delete(nni.podUIDToCandidateNodeMap, pod.UID)
	klog.V(6).InfoS("nrtcache: nni DEL ", "logID", klog.KObj(pod), "node", nodeName, "podUID", pod.UID, "wasReserved", wasReserved)
}

func (nni *nodeNameIndexer) insertPod(pod *corev1.Pod, nodeName string, obsState podObservedState) {
	pods, ok := nni.nodeToPodsMap[nodeName]
	if !ok {
		pods = make(map[types.UID]podData)
		nni.nodeToPodsMap[nodeName] = pods
	}
	pods[pod.UID] = podData{
		namespacedName: types.NamespacedName{
			Namespace: pod.GetNamespace(),
			Name:      pod.GetName(),
		},
		observedState: obsState,
	}
}

func (nni *nodeNameIndexer) getPodObservedState(pod *corev1.Pod, nodeName string) (podObservedState, bool) {
	pods, ok := nni.nodeToPodsMap[nodeName]
	if !ok {
		return podObservedBound, false
	}
	data, ok := pods[pod.UID]
	return data.observedState, ok
}

func namespacedNameListToString(nns []types.NamespacedName) string {
	var sb strings.Builder
	if len(nns) > 0 {
		sb.WriteString(nns[0].String())
	}
	for idx := 1; idx < len(nns); idx++ {
		sb.WriteString(", " + nns[idx].String())
	}
	return sb.String()
}
