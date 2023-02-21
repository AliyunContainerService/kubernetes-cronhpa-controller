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

package controller

import (
	"context"
	"fmt"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	autoscalingv1beta1 "github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
	"time"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// newReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	var stopChan chan struct{}
	cm := NewCronManager(mgr.GetConfig(), mgr.GetClient(), mgr.GetEventRecorderFor("CronHorizontalPodAutoscaler"))
	r := &ReconcileCronHorizontalPodAutoscaler{Client: mgr.GetClient(), scheme: mgr.GetScheme(), CronManager: cm}
	go func(cronManager *CronManager, stopChan chan struct{}) {
		cm.Run(stopChan)
		<-stopChan
	}(cm, stopChan)

	go func(cronManager *CronManager, stopChan chan struct{}) {
		server := NewWebServer(cronManager)
		server.serve()
	}(cm, stopChan)
	return r
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
	log.Infof("Start to handle cronHPA %s in %s namespace", request.Name, request.Namespace)
	instance := &autoscalingv1beta1.CronHorizontalPodAutoscaler{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			log.Infof("GC start for: cronHPA %s in %s namespace is not found", request.Name, request.Namespace)
			go r.CronManager.GC()
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	//log.Infof("%v is handled by cron-hpa controller", instance.Name)
	conditions := instance.Status.Conditions

	leftConditions := make([]v1beta1.Condition, 0)
	// check scaleTargetRef and excludeDates
	if checkGlobalParamsChanges(instance.Status, instance.Spec) {
		for _, cJob := range conditions {
			err := r.CronManager.delete(cJob.JobId)
			if err != nil {
				log.Errorf("Failed to delete job %s,because of %v", cJob.Name, err)
			}
		}
		// update scaleTargetRef and excludeDates
		instance.Status.ScaleTargetRef = instance.Spec.ScaleTargetRef
		instance.Status.ExcludeDates = instance.Spec.ExcludeDates
	} else {
		// check status and delete the expired job
		for _, cJob := range conditions {
			skip := false
			for _, job := range instance.Spec.Jobs {
				if cJob.Name == job.Name {
					// schedule has changed or RunOnce changed
					if cJob.Schedule != job.Schedule || cJob.RunOnce != job.RunOnce || cJob.TargetSize != job.TargetSize {
						// jobId exists and remove the job from cronManager
						if cJob.JobId != "" {
							err := r.CronManager.delete(cJob.JobId)
							if err != nil {
								log.Errorf("Failed to delete expired job %s,because of %v", cJob.Name, err)
							}
						}
						continue
					}
					// if nothing changed
					skip = true
				}
			}

			// need remove this condition because this is not job spec
			if !skip {
				if cJob.JobId != "" {
					err := r.CronManager.delete(cJob.JobId)
					if err != nil {
						log.Errorf("Failed to delete expired job %s,because of %v", cJob.Name, err)
					}
				}
			}

			// if job nothing changed then append to left conditions
			if skip {
				leftConditions = append(leftConditions, cJob)
			}
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
			RunOnce:       job.RunOnce,
			TargetSize:    job.TargetSize,
			LastProbeTime: metav1.Time{Time: time.Now()},
		}
		j, err := CronHPAJobFactory(instance, job, r.CronManager.scaler, r.CronManager.mapper, r.Client)

		if err != nil {
			jobCondition.State = v1beta1.Failed
			jobCondition.Message = fmt.Sprintf("Failed to create cron hpa job %s,because of %v", job.Name, err)
			log.Errorf("Failed to create cron hpa job %s,because of %v", job.Name, err)
		} else {
			name := job.Name
			if c, ok := leftConditionsMap[name]; ok {
				jobId := c.JobId
				j.SetID(jobId)

				// run once and return when reaches the final state
				if runOnce(job) && (c.State == v1beta1.Succeed || c.State == v1beta1.Failed) {
					err := r.CronManager.delete(jobId)
					if err != nil {
						log.Errorf("cron hpa %s(%s) has ran once but fail to exit,because of %v", name, jobId, err)
					}
					continue
				}
			}

			jobCondition.JobId = j.ID()
			err := r.CronManager.createOrUpdate(j)
			if err != nil {
				if _, ok := err.(*NoNeedUpdate); ok {
					continue
				} else {
					jobCondition.State = v1beta1.Failed
					jobCondition.Message = fmt.Sprintf("Failed to update cron hpa job %s,because of %v", job.Name, err)
				}
			} else {
				jobCondition.State = v1beta1.Submitted
			}
		}
		noNeedUpdateStatus = false
		instance.Status.Conditions = updateConditions(instance.Status.Conditions, jobCondition)
	}
	// conditions are not changed and no need to update.
	if !noNeedUpdateStatus || len(leftConditions) != len(conditions) {
		err := r.Update(context.Background(), instance)
		if err != nil {
			log.Errorf("Failed to update cron hpa %s in namespace %s status, because of %v", instance.Name, instance.Namespace, err)
		}
	}

	//log.Infof("%v has been handled completely.", instance)
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

// if global params changed then all jobs need to be recreated.
func checkGlobalParamsChanges(status v1beta1.CronHorizontalPodAutoscalerStatus, spec v1beta1.CronHorizontalPodAutoscalerSpec) bool {
	if &status.ScaleTargetRef != nil && (status.ScaleTargetRef.Kind != spec.ScaleTargetRef.Kind || status.ScaleTargetRef.ApiVersion != spec.ScaleTargetRef.ApiVersion ||
		status.ScaleTargetRef.Name != spec.ScaleTargetRef.Name) {
		return true
	}

	excludeDatesMap := make(map[string]bool)
	for _, date := range spec.ExcludeDates {
		excludeDatesMap[date] = true
	}

	for _, date := range status.ExcludeDates {
		if excludeDatesMap[date] {
			delete(excludeDatesMap, date)
		} else {
			return true
		}
	}
	// excludeMap change
	return len(excludeDatesMap) != 0
}

func runOnce(job v1beta1.Job) bool {
	if strings.Contains(job.Schedule, "@date ") || job.RunOnce {
		return true
	}
	return false
}
