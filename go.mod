module sigs.k8s.io/scheduler-plugins

go 1.16

require (
	github.com/dustin/go-humanize v1.0.0
	github.com/google/go-cmp v0.5.5
	github.com/k8stopologyawareschedwg/noderesourcetopology-api v0.0.12
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/paypal/load-watcher v0.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	gonum.org/v1/gonum v0.6.2
	k8s.io/api v0.23.3
	k8s.io/apimachinery v0.23.3
	k8s.io/apiserver v0.23.3
	k8s.io/client-go v0.23.3
	k8s.io/code-generator v0.23.3
	k8s.io/component-base v0.23.3
	k8s.io/component-helpers v0.23.3
	k8s.io/klog/hack/tools v0.0.0-20210917071902-331d2323a192
	k8s.io/klog/v2 v2.30.0
	k8s.io/kube-scheduler v0.23.3
	k8s.io/kubernetes v1.23.3
	k8s.io/utils v0.0.0-20211116205334-6203023598ed
	sigs.k8s.io/controller-runtime v0.11.1
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/spf13/cobra v1.2.1 // indirect
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5 // indirect
	golang.org/x/net v0.0.0-20211209124913-491a49abca63 // indirect
	golang.org/x/tools v0.1.6-0.20210820212750-d4cc65f0b2ff // indirect
	google.golang.org/genproto v0.0.0-20210831024726-fe130286e0e2 // indirect
	k8s.io/cloud-provider v0.23.3 // indirect
	k8s.io/csi-translation-lib v0.23.3 // indirect
	k8s.io/metrics v0.23.3 // indirect
	k8s.io/mount-utils v0.23.3 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.27 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.22.3
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.3
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.3
	k8s.io/apiserver => k8s.io/apiserver v0.22.3
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.22.3
	k8s.io/client-go => k8s.io/client-go v0.22.3
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.22.3
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.22.3
	k8s.io/code-generator => k8s.io/code-generator v0.22.3
	k8s.io/component-base => k8s.io/component-base v0.22.3
	k8s.io/component-helpers => k8s.io/component-helpers v0.22.3
	k8s.io/controller-manager => k8s.io/controller-manager v0.22.3
	k8s.io/cri-api => k8s.io/cri-api v0.22.3
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.22.3
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.8.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.3
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.22.3
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.22.3
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.22.3
	k8s.io/kubectl => k8s.io/kubectl v0.22.3
	k8s.io/kubelet => k8s.io/kubelet v0.22.3
	k8s.io/kubernetes => k8s.io/kubernetes v1.22.3
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.22.3
	k8s.io/metrics => k8s.io/metrics v0.22.3
	k8s.io/mount-utils => k8s.io/mount-utils v0.22.3
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.22.3
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.22.3
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.10.0
)
