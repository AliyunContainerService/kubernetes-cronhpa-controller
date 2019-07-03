package main

import (
	"flag"
	"fmt"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/client/clientset/versioned"
	"path/filepath"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset:=versioned.NewForConfigOrDie(config)
	if err!=nil{
		fmt.Println(err)
	}else {
		fmt.Println("connect with k8s")
	}
	instance:=&v1beta1.CronHorizontalPodAutoscaler{
		metav1.TypeMeta{"CronHorizontalPodAutoscaler","autoscaling.alibabacloud.com/v1beta1"},
		metav1.ObjectMeta{Name:"google"},
		v1beta1.CronHorizontalPodAutoscalerSpec {Jobs: []v1beta1.Job{}},
		v1beta1.CronHorizontalPodAutoscalerStatus{Conditions: []v1beta1.Condition{}},
	}
	//instance=&v1beta1.CronHorizontalPodAutoscaler{
	//	metav1.TypeMeta{"CronHorizontalPodAutoscaler","autoscaling.alibabacloud.com/v1beta1"},
	//	metav1.ObjectMeta{Name:"foo5"},
	//	v1beta1.CronHorizontalPodAutoscalerSpec {ScaleTargetRef:v1beta1.ScaleTargetRef{
	//		"apps/v1",
	//		"Deployment",
	//		"nginx-deployment-basic",
	//	},Jobs: []v1beta1.Job{{Name:"scale-down",Schedule:"30 */1 * * * *",TargetSize:1},{Name:"scale-up",Schedule:"0 */1 * * * *",TargetSize:3}}},
	//	//ScaleTargetRef:{ApiVersion:"apps/v1beta2",Kind:"Deployment",Name:"deployment-basic"}
	//	v1beta1.CronHorizontalPodAutoscalerStatus{Conditions: []v1beta1.Condition{}},
	//}
	cronhpa,err:=clientset.AutoscalingV1beta1().CronHorizontalPodAutoscalers("kube-system").Create(instance)
	if err!=nil{
		fmt.Println(err)
	}
	fmt.Println(cronhpa.Name)

}


