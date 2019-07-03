package v1beta1

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var HpaClientSet *kubernetes.Clientset
//init HPA Clientset..
func InitHpaClient(cfg *rest.Config)(err error){
	HpaClientSet,err=kubernetes.NewForConfig(cfg)
	if err!=nil{
		return err
	}
	return nil
}
