package v1beta1

import (
	"flag"
	"os"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"path/filepath"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"testing"
)
var cfg *rest.Config

func TestMain(m *testing.M) {
	//if os.Getenv("TRAVIS") != "" {
	//	return
	//}
	//t := &envtest.Environment{
	//	CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crds")},
	//}
	apis.AddToScheme(scheme.Scheme)
	var kubeconfig *string
	var err error
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	cfg, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}

	code := m.Run()
	//t.Stop()
	os.Exit(code)
}
