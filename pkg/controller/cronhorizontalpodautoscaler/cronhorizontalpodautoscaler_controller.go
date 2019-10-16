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
	"fmt"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	autoscalingv1beta1 "github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	log "github.com/Sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
	"time"
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
	var stopChan chan struct{}
	cm := NewCronManager(mgr.GetConfig(), mgr.GetClient(), mgr.GetRecorder("cron-horizontal-pod-autoscaler"))
	r := &ReconcileCronHorizontalPodAutoscaler{Client: mgr.GetClient(), scheme: mgr.GetScheme(), CronManager: cm}
	go func(cronManager *CronManager, stopChan chan struct{}) {
		cm.Run(stopChan)
		<-stopChan
	}(cm, stopChan)
	return r
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

	leftConditions := make([]v1beta1.Condition, 0)
	// check status and delete the expired job
	for _, cJob := range conditions {
		skip := false
		for _, job := range instance.Spec.Jobs {
			if cJob.Name == job.Name {
				// schedule has changed
				if cJob.Schedule != job.Schedule {
					// jobId exists and remove the job from cronManager
					if cJob.JobId != "" {
						err := r.CronManager.delete(cJob.JobId)
						if err != nil {
							log.Errorf("Failed to delete expired job %s,because of %s", cJob.Name, err.Error())
						}
					}
					continue
				}
				skip = true
			}
		}

		if skip == false {
			if cJob.JobId != "" {
				err := r.CronManager.delete(cJob.JobId)
				if err != nil {
					log.Errorf("Failed to delete expired job %s,because of %s", cJob.Name, err.Error())
				}
			}
		}

		// need remove this condition because this is not job spec
		if skip == true {
			leftConditions = append(leftConditions, cJob)
		}
	}

	// update the left to next step
	instance.Status.Conditions = leftConditions
	leftConditionsMap := convertConditionMaps(leftConditions)

	noNeedUpdateStatus := true
	for _, job := range instance.Spec.Jobs {
		jobCondition := v1beta1.Condition{
			Name:          job.Name,
			Schedule:      job.Schedule,
			LastProbeTime: metav1.Time{Time: time.Now()},
		}
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
		j, err := CronHPAJobFactory(ref, instance, job, r.CronManager.scaler, r.CronManager.mapper)

		if err != nil {
			jobCondition.State = v1beta1.Failed
			jobCondition.Message = fmt.Sprintf("Failed to create cron hpa job %s,because of %s", job.Name, err.Error())
			log.Errorf("Failed to create cron hpa job %s,because of %s", job.Name, err.Error())
		} else {
			name := job.Name
			if c, ok := leftConditionsMap[name]; ok {
				jobId := c.JobId
				j.SetID(jobId)
			}
			jobCondition.JobId = j.ID()
			err := r.CronManager.createOrUpdate(j)
			if err != nil {
				if _, ok := err.(*NoNeedUpdate); ok {
					continue
				} else {
					jobCondition.State = v1beta1.Failed
					jobCondition.Message = fmt.Sprintf("Failed to update cron hpa job %s,because of %s", job.Name, err.Error())
				}
			} else {
				jobCondition.State = v1beta1.Submitted
			}
		}
		noNeedUpdateStatus = false
		instance.Status.Conditions = updateConditions(instance.Status.Conditions, jobCondition)
	}
	// conditions doesn't changed and no need to update.
	if !noNeedUpdateStatus || len(leftConditions) != len(conditions) {
		err := r.Update(context.Background(), instance)
		if err != nil {
			log.Errorf("Failed to update cron hpa %s status,because of %s", instance.Name, err.Error())
		}
	}
	data,_:=json.Marshal(instance.Spec.ScaleTargetRef)
	annotations:=make(map[string]string)
	annotations["cronhpa.kubernetes.io"]=string(data)
	instance.Annotations=annotations
	if err:=r.Update(context.TODO(),instance);err!=nil{
		return reconcile.Result{},err
	}
	return reconcile.Result{}, nil
}

func convertConditionMaps(conditions []v1beta1.Condition) map[string]v1beta1.Condition {
	m := make(map[string]v1beta1.Condition)
	for _, condition := range conditions {
		m[condition.Name] = condition
	}
	return m
}

func updateConditions(conditions []v1beta1.Condition, condition v1beta1.Condition) []v1beta1.Condition {
	r := make([]v1beta1.Condition, 0)
	m := convertConditionMaps(conditions)
	m[condition.Name] = condition
	for _, condition := range m {
		r = append(r, condition)
	}
	return r
}
