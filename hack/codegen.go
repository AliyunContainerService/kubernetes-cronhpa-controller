package hack

// only for go mod vendor
// you can skip this error when running go test
//
//  go test -v -race github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/... \
//  github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/...
//
// would be much better.

import (
	_ "k8s.io/code-generator/cmd/client-gen"
	_ "k8s.io/code-generator/cmd/deepcopy-gen"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
