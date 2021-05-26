module github.com/AliyunContainerService/kubernetes-cronhpa-controller

go 1.14

require (
	github.com/go-logr/logr v0.2.1-0.20200730175230-ee2de8da5be6 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/ringtail/go-cron v1.0.1-0.20201027122514-cfb21c105f50
	github.com/satori/go.uuid v1.2.0
	k8s.io/api v0.19.0
	k8s.io/apiextensions-apiserver v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v0.19.0
	k8s.io/klog/v2 v2.2.0
	sigs.k8s.io/controller-runtime v0.6.2
)
