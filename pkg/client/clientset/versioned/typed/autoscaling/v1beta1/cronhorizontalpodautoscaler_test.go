package v1beta1

import (
	"context"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"testing"
)

func TestCronhpaCreate(t *testing.T){
	mgr, err :=manager.New(cfg,manager.Options{})
	if err!=nil{
		t.Errorf("create manager failed,err: %v",err)
	}
	client:=mgr.GetClient()
	//instance:=&v1beta1.CronHorizontalPodAutoscaler{
	//	metav1.TypeMeta{"CronHorizontalPodAutoscaler","autoscaling.alibabacloud.com/v1beta1"},
	//	metav1.ObjectMeta{Name:"go2",Namespace:"default"},
	//	v1beta1.CronHorizontalPodAutoscalerSpec {ScaleTargetRef: {ApiVersion:""  Kind:""  Name:"" },Jobs: []v1beta1.Job{}},
	//	v1beta1.CronHorizontalPodAutoscalerStatus{Conditions: []v1beta1.Condition{}},
	//}
	instance:=&v1beta1.CronHorizontalPodAutoscaler{
		metav1.TypeMeta{"CronHorizontalPodAutoscaler","autoscaling.alibabacloud.com/v1beta1"},
		metav1.ObjectMeta{Name:"foo1",Namespace:"kube-system"},
		v1beta1.CronHorizontalPodAutoscalerSpec {ScaleTargetRef:v1beta1.ScaleTargetRef{
			"apps/v1",
			"Deployment",
			"nginx-deployment-basic",
		},Jobs: []v1beta1.Job{{Name:"scale-down",Schedule:"30 */1 * * * *",TargetSize:1},{Name:"scale-up",Schedule:"0 */1 * * * *",TargetSize:3}}},
		//ScaleTargetRef:{ApiVersion:"apps/v1beta2",Kind:"Deployment",Name:"deployment-basic"}
		v1beta1.CronHorizontalPodAutoscalerStatus{Conditions: []v1beta1.Condition{}},
	}
	err=client.Create(context.TODO(),instance)
	if err!=nil{
		t.Errorf("create cronhpa failed,err: %v",err)
	}
}
