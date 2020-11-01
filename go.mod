module github.com/AliyunContainerService/kubernetes-cronhpa-controller

go 1.14

require (
	github.com/go-logr/logr v0.2.1-0.20200730175230-ee2de8da5be6 // indirect
	github.com/go-logr/zapr v0.2.0 // indirect; indrect
	github.com/googleapis/gnostic v0.5.1 // indirect
	github.com/onsi/gomega v1.10.1
	github.com/ringtail/go-cron v1.0.1-0.20201027122514-cfb21c105f50
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.6.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	k8s.io/api v0.19.0
	k8s.io/apiextensions-apiserver v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v0.19.0
	k8s.io/code-generator v0.19.0
	sigs.k8s.io/controller-runtime v0.6.2
	sigs.k8s.io/controller-tools v0.4.0
)
