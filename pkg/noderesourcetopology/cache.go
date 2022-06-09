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
	"errors"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
	"github.com/k8stopologyawareschedwg/podfingerprint"
)

type Cache interface {
	GetByNode(nodeName string) *topologyv1alpha1.NodeResourceTopology
	MarkNodeDiscarded(nodeName string)
	ReserveNodeResources(nodeName string, pod *corev1.Pod)
	ReleaseNodeResources(nodeName string, pod *corev1.Pod)
}

type PassthroughCache struct {
	lister listerv1alpha1.NodeResourceTopologyLister
}

func (pt PassthroughCache) GetByNode(nodeName string) *topologyv1alpha1.NodeResourceTopology {
	klog.V(5).InfoS("Lister for nodeResTopoPlugin", "lister", pt.lister)
	nrt, err := pt.lister.Get(nodeName)
	if err != nil {
		klog.V(5).ErrorS(err, "Cannot get NodeTopologies from NodeResourceTopologyLister")
		return nil
	}
	return nrt
}

func (pt PassthroughCache) MarkNodeDiscarded(nodeName string)                     {}
func (pt PassthroughCache) ReserveNodeResources(nodeName string, pod *corev1.Pod) {}
func (pt PassthroughCache) ReleaseNodeResources(nodeName string, pod *corev1.Pod) {}

// this is only to make it trivial to test, cache.Indexer works just fine
type IndexGetter interface {
	ByIndex(indexName, indexedValue string) ([]interface{}, error)
}

type NRTCache struct {
	lock             sync.RWMutex
	nrts             nrtStore
	assumedResources map[string]resourceStore
	nodeDiscarded    counter
	lister           listerv1alpha1.NodeResourceTopologyLister
	indexer          IndexGetter
}

func newNRTCache(lister listerv1alpha1.NodeResourceTopologyLister, indexer IndexGetter) (*NRTCache, error) {
	if lister == nil || indexer == nil {
		return nil, fmt.Errorf("NRT cache received nil references")
	}

	nrtObjs, err := lister.List(labels.Nothing())
	if err != nil {
		return nil, err
	}
	return &NRTCache{
		nrts:             newNrtStore(nrtObjs),
		assumedResources: make(map[string]resourceStore),
		nodeDiscarded:    make(counter),
		lister:           lister,
		indexer:          indexer,
	}, nil
}

func (cc *NRTCache) DirtyNodeNames() []string {
	cc.lock.RLock()
	defer cc.lock.RUnlock()
	var nodes []string
	for node := range cc.assumedResources {
		if !cc.nodeDiscarded.IsSet(node) {
			// noone asked about this node, let it be
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func (cc *NRTCache) MarkNodeDiscarded(nodeName string) {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	val := cc.nodeDiscarded.Incr(nodeName)
	klog.V(6).InfoS("discarded", "node", nodeName, "count", val)
}

func (cc *NRTCache) GetByNode(nodeName string) *topologyv1alpha1.NodeResourceTopology {
	cc.lock.RLock()
	defer cc.lock.RUnlock()
	nrt := cc.nrts.GetByNode(nodeName)
	if nrt == nil {
		return nil
	}
	nodeAssumedResources := cc.assumedResources[nodeName]
	nodeAssumedResources.UpdateNRT(nrt)
	return nrt
}

func (cc *NRTCache) ReserveNodeResources(nodeName string, pod *corev1.Pod) {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	nodeAssumedResources, ok := cc.assumedResources[nodeName]
	if !ok {
		nodeAssumedResources = make(resourceStore)
		cc.assumedResources[nodeName] = nodeAssumedResources
	}

	nodeAssumedResources.AddPod(pod)

	cc.nodeDiscarded.Delete(nodeName)
	klog.V(6).InfoS("reset discard counter", "node", nodeName)
}

func (cc *NRTCache) ReleaseNodeResources(nodeName string, pod *corev1.Pod) {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	nodeAssumedResources, ok := cc.assumedResources[nodeName]
	if !ok {
		// this should not happen, so we're vocal about it
		// we don't return error because not much to do to recover anyway
		klog.V(3).InfoS("no resources tracked", "node", nodeName)
	}
	nodeAssumedResources.DeletePod(pod)
}

func (cc *NRTCache) Resync() {
	klog.V(6).Infof("resyncing NodeTopology cache")

	nodeNames := cc.DirtyNodeNames()

	var nrtUpdates []*topologyv1alpha1.NodeResourceTopology
	for _, nodeName := range nodeNames {
		nrtCandidate, err := cc.lister.Get(nodeName)
		if err != nil || nrtCandidate == nil {
			klog.V(3).InfoS("missing NodeTopology", "node", nodeName)
			continue
		}

		pfpExpected := podFingerprintForNodeTopology(nrtCandidate)
		if pfpExpected == "" {
			klog.V(3).InfoS("missing NodeTopology podset fingerprint data", "node", nodeName)
			continue
		}

		klog.V(6).InfoS("trying to resync NodeTopology", "node", nodeName, "fingerprint", pfpExpected)

		err = checkPodFingerprintForNode(cc.indexer, nodeName, pfpExpected)
		if errors.Is(err, podfingerprint.ErrSignatureMismatch) {
			// can happen, not critical
			klog.V(6).InfoS("NodeTopology podset fingerprint mismatch", "node", nodeName)
			continue
		}
		if err != nil {
			// should never happen, let's be vocal
			klog.V(3).ErrorS(err, "checking NodeTopology podset fingerprint", "node", nodeName)
			continue
		}

		nrtUpdates = append(nrtUpdates, nrtCandidate)
	}

	cc.FlushNodes(nrtUpdates...)

	klog.V(6).Infof("resynced NodeTopology cache")
}

func (cc *NRTCache) FlushNodes(nrts ...*topologyv1alpha1.NodeResourceTopology) {
	cc.lock.Lock()
	defer cc.lock.Unlock()
	for _, nrt := range nrts {
		klog.V(6).InfoS("flushing", "node", nrt.Name)
		cc.nrts.Update(nrt)
		delete(cc.assumedResources, nrt.Name)
		cc.nodeDiscarded.Delete(nrt.Name)
	}
}

// to be used only in tests
func (cc *NRTCache) Store() *nrtStore {
	return &cc.nrts
}
