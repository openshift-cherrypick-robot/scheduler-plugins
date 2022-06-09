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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
)

type Cache interface {
	GetByNode(nodeName string) *topologyv1alpha1.NodeResourceTopology
	MarkNodeDiscarded(nodeName string)
	ReserveNodeResources(nodeName string, pod *corev1.Pod) error
	ReleaseNodeResources(nodeName string, pod *corev1.Pod) error
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

func (pt PassthroughCache) MarkNodeDiscarded(nodeName string)                           {}
func (pt PassthroughCache) ReserveNodeResources(nodeName string, pod *corev1.Pod) error { return nil }
func (pt PassthroughCache) ReleaseNodeResources(nodeName string, pod *corev1.Pod) error { return nil }
