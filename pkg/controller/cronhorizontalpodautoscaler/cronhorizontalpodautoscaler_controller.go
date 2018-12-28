/*
Copyright 2018 zhongwei.lzw@alibaba-inc.com.

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

package cronhorizontalpodautoscaler

import (
	"context"
	autoscalingv1beta1 "gitlab.alibaba-inc.com/cos/kubernetes-cron-hpa-controller/pkg/apis/autoscaling/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	log "github.com/Sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"gitlab.alibaba-inc.com/cos/kubernetes-cron-hpa-controller/pkg/apis/autoscaling/v1beta1"
	"strings"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new CronHorizontalPodAutoscaler Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this autoscaling.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCronHorizontalPodAutoscaler{Client: mgr.GetClient(), scheme: mgr.GetScheme(), CronManager: NewCronManager(mgr.GetConfig(), mgr.GetClient(), mgr.GetRecorder("cron-horizontal-pod-autoscaler")),}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cron-horizontal-pod-autoscaler-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to CronHorizontalPodAutoscaler
	err = c.Watch(&source.Kind{Type: &autoscalingv1beta1.CronHorizontalPodAutoscaler{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	go func() {
		var stopChan chan struct{}

		cm := r.(*ReconcileCronHorizontalPodAutoscaler).CronManager
		cm.Run(stopChan)
		<-stopChan

	}()
	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by CronHorizontalPodAutoscaler - change this for objects you create
	//err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
	//	IsController: true,
	//	OwnerType:    &autoscalingv1beta1.CronHorizontalPodAutoscaler{},
	//})
	//if err != nil {
	//	return err
	//}
	return nil
}

var _ reconcile.Reconciler = &ReconcileCronHorizontalPodAutoscaler{}

// ReconcileCronHorizontalPodAutoscaler reconciles a CronHorizontalPodAutoscaler object
type ReconcileCronHorizontalPodAutoscaler struct {
	client.Client
	scheme      *runtime.Scheme
	CronManager *CronManager
}

// Reconcile reads that state of the cluster for a CronHorizontalPodAutoscaler object and makes changes based on the state read
// and what is in the CronHorizontalPodAutoscaler.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.alibabacloud.com,resources=cronhorizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileCronHorizontalPodAutoscaler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CronHorizontalPodAutoscaler instance
	instance := &autoscalingv1beta1.CronHorizontalPodAutoscaler{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			go r.CronManager.GC()
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	conditions := instance.Status.Conditions

	for index, condition := range conditions {
		needClean := true
		for _, job := range instance.Spec.Jobs {
			if condition.Name == job.Name {
				needClean = false
				break;
			}
		}
		if needClean {
			instance.Status.Conditions = append(instance.Status.Conditions[0:index], instance.Status.Conditions[index+1:]...)
		}
	}

	conditions = instance.Status.Conditions
	for _, job := range instance.Spec.Jobs {
		isSubmit := false
		arr := strings.Split(instance.Spec.ScaleTargetRef.ApiVersion, "/")
		group := arr[0]
		version := arr[1]
		ref := &TargetRef{
			RefName:      instance.Spec.ScaleTargetRef.Name,
			RefNamespace: instance.Namespace,
			RefKind:      instance.Spec.ScaleTargetRef.Kind,
			RefGroup:     group,
			RefVersion:   version,
		}
		j, err := CronHPAJobFactory(ref, instance, job.Name, job.Schedule, job.TargetSize, r.CronManager.scaler, r.CronManager.mapper)
		for _, condition := range conditions {
			if condition.Name == job.Name {
				// mark as a old job update
				j.SetID(condition.JobId)
				if err != nil {
					log.Errorf("Failed to convert job from cronHPA and skip,because of %s.", err.Error())
					continue
				}
				r.CronManager.update(j)
				isSubmit = true
			}
		}
		if isSubmit == false {
			r.CronManager.update(j)
			isSubmit = true
			c := &v1beta1.Condition{
				Name:  job.Name,
				JobId: j.ID(),
				State: v1beta1.Submitted,
			}
			instance.Status.Conditions = append(instance.Status.Conditions, *c)
		}

		r.Update(context.Background(), instance)
	}
	return reconcile.Result{}, nil
}
