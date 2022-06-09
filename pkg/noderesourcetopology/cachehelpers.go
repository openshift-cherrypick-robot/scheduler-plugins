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

package noderesourcetopology

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"github.com/k8stopologyawareschedwg/podfingerprint"
)

var zeroQty resource.Quantity

func init() {
	zeroQty = resource.MustParse("0")
}

type nrtStore map[string]*topologyv1alpha1.NodeResourceTopology

func newNrtStore(nrts []*topologyv1alpha1.NodeResourceTopology) nrtStore {
	data := make(map[string]*topologyv1alpha1.NodeResourceTopology)
	for _, nrt := range nrts {
		data[nrt.Name] = nrt.DeepCopy()
	}
	return data
}

func (nrs nrtStore) GetByNode(nodeName string) *topologyv1alpha1.NodeResourceTopology {
	obj, ok := nrs[nodeName]
	if !ok {
		klog.V(5).InfoS("missing cached NodeTopology", "node", nodeName)
		return nil
	}
	return obj.DeepCopy()
}

func (nrs nrtStore) Update(nrt *topologyv1alpha1.NodeResourceTopology) {
	nrs[nrt.Name] = nrt.DeepCopy()
	klog.V(5).InfoS("updated cached NodeTopology", "node", nrt.Name)
}

type resourceStore map[string]corev1.ResourceList

// AddPod returns true if updating existing pod, false if adding for the first time
func (rs resourceStore) AddPod(pod *corev1.Pod) bool {
	key, ok := rs.ContainsPod(pod)
	if ok {
		// should not happen, so we log with a low level
		klog.V(4).InfoS("updating existing entry", "key", key)
	}
	rs[key] = resourceCountersFromPod(pod)
	return ok
}

// DeletePod returns true if deleted an existing pod, false otherwise
func (rs resourceStore) DeletePod(pod *corev1.Pod) bool {
	key, ok := rs.ContainsPod(pod)
	if !ok {
		// should not happen, so we log with a low level
		klog.V(4).InfoS("removing missing entry", "key", key)
	}
	delete(rs, key)
	return ok
}

func (rs resourceStore) ContainsPod(pod *corev1.Pod) (string, bool) {
	key := resourceStoreKey(pod)
	_, ok := rs[key]
	return key, ok
}

func (rs resourceStore) UpdateNRT(nrt *topologyv1alpha1.NodeResourceTopology) {
	for key, res := range rs {
		// We cannot predict on which Zone the workload will be placed.
		// And we should totally not guess. So the only safe (and conservative)
		// choice is to decrement the available resources from *all* the zones.
		// This can cause false negatives, but will never cause false positives,
		// which are much worse.
		for zi := 0; zi < len(nrt.Zones); zi++ {
			zone := &nrt.Zones[zi] // shortcut
			for ri := 0; ri < len(zone.Resources); ri++ {
				zr := &zone.Resources[ri] // shortcut
				qty, ok := res[corev1.ResourceName(zr.Name)]
				if !ok {
					// this is benign; it is totally possible some resources are not
					// available on some zones (think PCI devices), hence we don't
					// even report this error, being an expected condition
					continue
				}
				if zr.Available.Cmp(qty) < 0 {
					// this should happen rarely, and it is likely caused by
					// a bug elsewhere.
					klog.V(6).InfoS("cannot decrement resource", "zone", zr.Name, "node", nrt.Name, "available", zr.Available, "requestor", key, "quantity", qty)
					zr.Available = zeroQty
					continue
				}

				zr.Available.Sub(qty)
			}
		}
	}
}

func resourceStoreKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

type counter map[string]int

func (cnt counter) Incr(key string) int {
	val := cnt[key]
	val++
	cnt[key] = val
	return val
}

func (cnt counter) IsSet(key string) bool {
	return cnt[key] > 0
}

func (cnt counter) Delete(key string) {
	delete(cnt, key)
}

func indexByPodNodeName(obj interface{}) ([]string, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return []string{}, nil
	}
	if len(pod.Spec.NodeName) == 0 {
		// TODO: further restrict with pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed
		return []string{}, nil
	}
	return []string{pod.Spec.NodeName}, nil
}

func podFingerprintForNodeTopology(nrt *topologyv1alpha1.NodeResourceTopology) string {
	if nrt.Annotations == nil {
		return ""
	}
	return nrt.Annotations[podfingerprint.Annotation]
}

func checkPodFingerprintForNode(indexer IndexGetter, nodeName, pfpExpected string) error {
	objs, err := indexer.ByIndex(NodeNameIndexer, nodeName)
	if err != nil {
		return err
	}
	pfp := podfingerprint.NewFingerprint(len(objs))
	for _, obj := range objs {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return fmt.Errorf("non-pod object from indexer: %#v", obj)
		}
		pfp.AddPod(pod)
	}
	klog.V(6).InfoS("podset fingerprint check", "node", nodeName, "expected", pfpExpected, "computed", pfp.Sign())
	return pfp.Check(pfpExpected)
}

func resourceCountersFromPod(pod *corev1.Pod) corev1.ResourceList {
	rc := make(corev1.ResourceList)
	if pod == nil {
		return rc
	}
	for idx := 0; idx < len(pod.Spec.Containers); idx++ {
		for resName, resQty := range pod.Spec.Containers[idx].Resources.Requests {
			qty := rc[resName]
			qty.Add(resQty)
			rc[resName] = qty
		}
	}
	return rc
}
