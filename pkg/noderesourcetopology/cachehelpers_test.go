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
	"testing"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/k8stopologyawareschedwg/podfingerprint"
)

func TestIndexByPodNodeName(t *testing.T) {
	vals, _ := indexByPodNodeName(42)
	if len(vals) != 0 {
		t.Errorf("indexed unsupported type")
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-0",
			Name:      "pod-0",
		},
		// not non-inited PodSpec
	}
	vals, _ = indexByPodNodeName(&pod)
	if len(vals) != 0 {
		t.Errorf("indexed pod with empty spec")
	}

	nodeName := "test-node"
	pod.Spec.NodeName = nodeName
	vals, _ = indexByPodNodeName(&pod)
	if vals[0] != nodeName {
		t.Errorf("expected index %q got %q", nodeName, vals[0])
	}
}

func TestFingerprintFromNRT(t *testing.T) {
	nrt := &topologyv1alpha1.NodeResourceTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-0",
		},
		TopologyPolicies: []string{
			"best-effort",
		},
	}

	var pfp string
	pfp = podFingerprintForNodeTopology(nrt)
	if pfp != "" {
		t.Errorf("misdetected fingerprint from missing annotations")
	}

	nrt.Annotations = map[string]string{}
	pfp = podFingerprintForNodeTopology(nrt)
	if pfp != "" {
		t.Errorf("misdetected fingerprint from empty annotations")
	}

	pfpTest := "test"
	nrt.Annotations[podfingerprint.Annotation] = pfpTest
	pfp = podFingerprintForNodeTopology(nrt)
	if pfp != pfpTest {
		t.Errorf("misdetected fingerprint as %q expected %q", pfp, pfpTest)
	}
}

func TestNRTStoreGet(t *testing.T) {
	nrts := []*topologyv1alpha1.NodeResourceTopology{
		&topologyv1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-0",
			},
			TopologyPolicies: []string{
				"best-effort",
			},
		},
		&topologyv1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			TopologyPolicies: []string{
				"restricted",
			},
		},
	}
	ns := newNrtStore(nrts)

	obj := ns.GetByNode("node-0")
	obj.TopologyPolicies[0] = "single-numa-node"

	obj2 := ns.GetByNode("node-0")
	if obj2.TopologyPolicies[0] != nrts[0].TopologyPolicies[0] {
		t.Errorf("change to local copy propagated back in the store")
	}

	nrts[0].TopologyPolicies[0] = "single-numa-node"
	obj2 = ns.GetByNode("node-0")
	if obj2.TopologyPolicies[0] != "best-effort" { // original value when the object was first added to the store
		t.Errorf("stored value is not an independent copy")
	}
}

func TestNRTStoreUpdate(t *testing.T) {
	nrts := []*topologyv1alpha1.NodeResourceTopology{
		&topologyv1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-0",
			},
			TopologyPolicies: []string{
				"best-effort",
			},
		},
		&topologyv1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			TopologyPolicies: []string{
				"restricted",
			},
		},
	}
	ns := newNrtStore(nrts)

	nrt3 := &topologyv1alpha1.NodeResourceTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-2",
		},
		TopologyPolicies: []string{
			"none",
		},
	}
	ns.Update(nrt3)
	nrt3.TopologyPolicies[0] = "best-effort"

	obj3 := ns.GetByNode("node-2")
	if obj3.TopologyPolicies[0] != "none" { // original value when the object was first added to the store
		t.Errorf("stored value is not an independent copy")
	}
}

func TestNRTStoreGetMissing(t *testing.T) {
	ns := newNrtStore(nil)
	if ns.GetByNode("node-missing") != nil {
		t.Errorf("missing node returned non-nil data")
	}
}

func TestCounterIncr(t *testing.T) {
	cnt := make(counter)

	if cnt.IsSet("missing") {
		t.Errorf("found nonexisting key in empty counter")
	}

	cnt.Incr("aaa")
	cnt.Incr("aaa")
	if val := cnt.Incr("aaa"); val != 3 {
		t.Errorf("unexpected counter value: %d expected %d", val, 3)
	}
	cnt.Incr("bbb")

	if !cnt.IsSet("aaa") {
		t.Errorf("missing expected key: %q", "aaa")
	}
	if !cnt.IsSet("bbb") {
		t.Errorf("missing expected key: %q", "bbb")
	}
}

func TestCounterDelete(t *testing.T) {
	cnt := make(counter)

	cnt.Incr("aaa")
	cnt.Incr("aaa")
	cnt.Incr("bbb")

	cnt.Delete("aaa")
	if cnt.IsSet("aaa") {
		t.Errorf("found unexpected key: %q", "aaa")
	}
	if !cnt.IsSet("bbb") {
		t.Errorf("missing expected key: %q", "bbb")
	}
}

func TestFromPodWithEmptyValue(t *testing.T) {
	rc1 := resourceCountersFromPod(nil)
	if len(rc1) > 0 {
		t.Fatalf("non-zero resource counters from nil pod")
	}

	rc2 := resourceCountersFromPod(&corev1.Pod{})
	if len(rc2) > 0 {
		t.Fatalf("non-zero resource counters from zero pod")
	}
}

const (
	nicName = "vendor_A.com/nic"
	gpuName = "vendor_B.com/gpu"
)

func TestFromPodWithMultipleContainers(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-0",
			Name:      "pod-0",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cnt-0",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:           resource.MustParse("4"),
							corev1.ResourceMemory:        resource.MustParse("2Gi"),
							corev1.ResourceName(nicName): resource.MustParse("2"),
						},
					},
				},
				{
					Name: "cnt-1",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:           resource.MustParse("1"),
							corev1.ResourceMemory:        resource.MustParse("1Gi"),
							corev1.ResourceName(gpuName): resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	rc := resourceCountersFromPod(&pod)
	if len(rc) == 0 {
		t.Fatalf("missing resource counters from non-empty pod")
	}

	if cpus := rc[corev1.ResourceCPU]; cpus.Cmp(resource.MustParse("5")) != 0 {
		t.Errorf("unexpected resource %q: desired %v got %v", corev1.ResourceCPU, "5", cpus)
	}
	if mems := rc[corev1.ResourceMemory]; mems.Cmp(resource.MustParse("3Gi")) != 0 {
		t.Errorf("unexpected resource %q: desired %v got %v", corev1.ResourceMemory, "3Gi", mems)
	}
	if gpus := rc[corev1.ResourceName(gpuName)]; gpus.Cmp(resource.MustParse("1")) != 0 {
		t.Errorf("unexpected resource %q: desired %v got %v", gpuName, "1", gpus)
	}
	if nics := rc[corev1.ResourceName(nicName)]; nics.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("unexpected resource %q: desired %v got %v", nicName, "2", nics)
	}
}

func TestResourceStoreAddPod(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-0",
			Name:      "pod-0",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cnt-0",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("16"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
	}

	rs := make(resourceStore)
	existed := rs.AddPod(&pod)
	if existed {
		t.Fatalf("replaced a pod into a empty resourceStore")
	}
	existed = rs.AddPod(&pod)
	if !existed {
		t.Fatalf("added pod twice")
	}
}

func TestResourceStoreDeletePod(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-0",
			Name:      "pod-0",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cnt-0",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("16"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
	}

	rs := make(resourceStore)
	existed := rs.DeletePod(&pod)
	if existed {
		t.Fatalf("deleted a pod into a empty resourceStore")
	}
	rs.AddPod(&pod)
	existed = rs.DeletePod(&pod)
	if !existed {
		t.Fatalf("deleted a pod which was not supposed to be present")
	}
}

func TestResourceStoreUpdate(t *testing.T) {
	nrt := &topologyv1alpha1.NodeResourceTopology{
		ObjectMeta:       metav1.ObjectMeta{Name: "node"},
		TopologyPolicies: []string{string(topologyv1alpha1.SingleNUMANodePodLevel)},
		Zones: topologyv1alpha1.ZoneList{
			{
				Name: "node-0",
				Type: "Node",
				Resources: topologyv1alpha1.ResourceInfoList{
					MakeTopologyResInfo(cpu, "20", "20"),
					MakeTopologyResInfo(memory, "32Gi", "32Gi"),
				},
			},
			{
				Name: "node-1",
				Type: "Node",
				Resources: topologyv1alpha1.ResourceInfoList{
					MakeTopologyResInfo(cpu, "20", "20"),
					MakeTopologyResInfo(memory, "32Gi", "32Gi"),
					MakeTopologyResInfo(nicName, "8", "8"),
				},
			},
		},
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-0",
			Name:      "pod-0",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "cnt-0",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:           resource.MustParse("16"),
							corev1.ResourceMemory:        resource.MustParse("4Gi"),
							corev1.ResourceName(nicName): resource.MustParse("2"),
						},
					},
				},
				{
					Name: "cnt-1",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
		},
	}

	rs := make(resourceStore)
	existed := rs.AddPod(&pod)
	if existed {
		t.Fatalf("replacing a pod into a empty resourceStore")
	}
	rs.UpdateNRT(nrt)

	cpuInfo0 := findResourceInfo(nrt.Zones[0].Resources, cpu)
	if cpuInfo0.Capacity.Cmp(resource.MustParse("20")) != 0 {
		t.Errorf("bad capacity for resource %q on zone %d: expected %v got %v", cpu, 0, "20", cpuInfo0.Capacity)
	}
	if cpuInfo0.Available.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("bad availability for resource %q on zone %d: expected %v got %v", cpu, 0, "2", cpuInfo0.Available)
	}
	cpuInfo1 := findResourceInfo(nrt.Zones[1].Resources, cpu)
	if cpuInfo1.Capacity.Cmp(resource.MustParse("20")) != 0 {
		t.Errorf("bad capacity for resource %q on zone %d: expected %v got %v", cpu, 1, "20", cpuInfo1.Capacity)
	}
	if cpuInfo1.Available.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("bad availability for resource %q on zone %d: expected %v got %v", cpu, 1, "2", cpuInfo1.Available)
	}

	memInfo0 := findResourceInfo(nrt.Zones[0].Resources, memory)
	if memInfo0.Capacity.Cmp(resource.MustParse("32Gi")) != 0 {
		t.Errorf("bad capacity for resource %q on zone %d: expected %v got %v", memory, 0, "32Gi", memInfo0.Capacity)
	}
	if memInfo0.Available.Cmp(resource.MustParse("26Gi")) != 0 {
		t.Errorf("bad availability for resource %q on zone %d: expected %v got %v", memory, 0, "26Gi", memInfo0.Available)
	}
	memInfo1 := findResourceInfo(nrt.Zones[1].Resources, memory)
	if memInfo1.Capacity.Cmp(resource.MustParse("32Gi")) != 0 {
		t.Errorf("bad capacity for resource %q on zone %d: expected %v got %v", memory, 1, "32Gi", memInfo1.Capacity)
	}
	if memInfo1.Available.Cmp(resource.MustParse("26Gi")) != 0 {
		t.Errorf("bad availability for resource %q on zone %d: expected %v got %v", memory, 1, "26Gi", memInfo1.Available)
	}

	devInfo0 := findResourceInfo(nrt.Zones[0].Resources, nicName)
	if devInfo0 != nil {
		t.Errorf("unexpected device %q on zone %d", nicName, 0)
	}

	devInfo1 := findResourceInfo(nrt.Zones[1].Resources, nicName)
	if devInfo1 == nil {
		t.Fatalf("expected device %q on zone %d, but missing", nicName, 1)
	}
	if devInfo1.Capacity.Cmp(resource.MustParse("8")) != 0 {
		t.Errorf("bad capacity for resource %q on zone %d: expected %v got %v", nicName, 1, "8", devInfo1.Capacity)
	}
	if devInfo1.Available.Cmp(resource.MustParse("6")) != 0 {
		t.Errorf("bad availability for resource %q on zone %d: expected %v got %v", nicName, 1, "6", devInfo1.Available)
	}
}

func findResourceInfo(rinfos []topologyv1alpha1.ResourceInfo, name string) *topologyv1alpha1.ResourceInfo {
	for idx := 0; idx < len(rinfos); idx++ {
		if rinfos[idx].Name == name {
			return &rinfos[idx]
		}
	}
	return nil
}
