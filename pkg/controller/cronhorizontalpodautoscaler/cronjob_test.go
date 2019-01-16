package cronhorizontalpodautoscaler

//
//import (
//	"testing"
//	"k8s.io/client-go/restmapper"
//	cacheddiscovery "k8s.io/client-go/discovery/cached"
//	"k8s.io/client-go/kubernetes"
//	"k8s.io/client-go/dynamic"
//	"k8s.io/client-go/scale"
//	"k8s.io/client-go/rest"
//	"k8s.io/client-go/tools/clientcmd"
//	"os"
//	log "github.com/Sirupsen/logrus"
//	"k8s.io/client-go/informers"
//	"k8s.io/kubernetes/pkg/controller"
//	"k8s.io/apimachinery/pkg/util/wait"
//	"time"
//)
//
//var (
//	kubeconf = os.Getenv("kubeconf")
//)
//
//var cli kubernetes.Interface
//var config *rest.Config
//var err error
//
//func init() {
//	config, err = clientcmd.BuildConfigFromFlags("", kubeconf)
//	if err != nil {
//		log.Error(err)
//		return
//	}
//	cli, err = kubernetes.NewForConfig(config)
//	if err != nil {
//		log.Error(err)
//		return
//	}
//}
//
//func TestCronJobScale(t *testing.T) {
//
//	var stopChan chan struct{}
//	rootClientBuilder := controller.SimpleControllerClientBuilder{
//		ClientConfig: config,
//	}
//
//	versionedClient := rootClientBuilder.ClientOrDie("shared-informers")
//	sharedInformers := informers.NewSharedInformerFactory(versionedClient, time.Second*120)
//
//	// Use a discovery client capable of being refreshed.
//	discoveryClient := rootClientBuilder.ClientOrDie("controller-discovery")
//	cachedClient := cacheddiscovery.NewMemCacheClient(discoveryClient.Discovery())
//	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedClient)
//	go wait.Until(func() {
//		restMapper.Reset()
//	}, 30*time.Second, stopChan)
//
//	sharedInformers.Start(stopChan)
//
//	c := &CronJobHPA{
//		id:           "1",
//		RefName:      "nginx-deployment-basic",
//		RefNamespace: "default",
//		RefKind:      "Deployment",
//		RefGroup:     "apps",
//		RefVersion:   "v1beta2",
//		DesiredSize:  5,
//		mapper:       restMapper,
//	}
//	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(rootClientBuilder.ClientOrDie("horizontal-pod-autoscaler").Discovery())
//	scaler, err := scale.NewForConfig(rootClientBuilder.ConfigOrDie("horizontal-pod-autoscaler"), c.mapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
//	c.scaler = scaler
//	if err != nil {
//		t.Errorf("Failed to create scaler,because of %s", err.Error())
//		return
//	}
//	c.Run()
//	t.Log("success")
//}
