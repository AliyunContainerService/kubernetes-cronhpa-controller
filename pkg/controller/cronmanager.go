package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ringtail/go-cron"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1beta1 "github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	scalelib "github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/lib"
)

const (
	MaxRetryTimes = 3
	GCInterval    = 10 * time.Minute
)

type NoNeedUpdate struct{}

func (n NoNeedUpdate) Error() string {
	return "NoNeedUpdate"
}

type CronManager struct {
	sync.Mutex
	cfg      *rest.Config
	client   client.Client
	jobQueue map[string]CronJob
	//cronProcessor CronProcessor
	cronExecutor  CronExecutor
	mapper        meta.RESTMapper
	scaler        scale.ScalesGetter
	eventRecorder record.EventRecorder
}

func (cm *CronManager) createOrUpdate(j CronJob) error {
	cm.Lock()
	defer cm.Unlock()
	if _, ok := cm.jobQueue[j.ID()]; !ok {
		err := cm.cronExecutor.AddJob(j)
		if err != nil {
			return fmt.Errorf("Failed to add job to cronExecutor,because of %v", err)
		}
		cm.jobQueue[j.ID()] = j
		log.Infof("cronHPA job %s of cronHPA %s in %s created, %d active jobs exist", j.Name(), j.CronHPAMeta().Name, j.CronHPAMeta().Namespace,
			len(cm.jobQueue))
	} else {
		job := cm.jobQueue[j.ID()]
		if ok := job.Equals(j); !ok {
			err := cm.cronExecutor.Update(j)
			if err != nil {
				return fmt.Errorf("failed to update job %s of cronHPA %s in %s to cronExecutor, because of %v", job.Name(), job.CronHPAMeta().Name, job.CronHPAMeta().Namespace, err)
			}
			//update job queue
			cm.jobQueue[j.ID()] = j
			log.Infof("cronHPA job %s of cronHPA %s in %s updated, %d active jobs exist", j.Name(), j.CronHPAMeta().Name, j.CronHPAMeta().Namespace, len(cm.jobQueue))
		} else {
			return &NoNeedUpdate{}
		}
	}
	return nil
}

func (cm *CronManager) delete(id string) error {
	cm.Lock()
	defer cm.Unlock()
	if j, ok := cm.jobQueue[id]; ok {
		err := cm.cronExecutor.RemoveJob(j)
		if err != nil {
			return fmt.Errorf("Failed to remove job from cronExecutor,because of %v", err)
		}
		delete(cm.jobQueue, id)
		log.Infof("Remove cronHPA job %s of cronHPA %s in %s from jobQueue,%d active jobs left", j.Name(), j.CronHPAMeta().Name, j.CronHPAMeta().Namespace, len(cm.jobQueue))
	}
	return nil
}

func (cm *CronManager) JobResultHandler(js *cron.JobResult) {
	job := js.Ref.(*CronJobHPA)
	cronHpa := js.Ref.(*CronJobHPA).HPARef
	instance := &autoscalingv1beta1.CronHorizontalPodAutoscaler{}
	e := cm.client.Get(context.TODO(), types.NamespacedName{
		Namespace: cronHpa.Namespace,
		Name:      cronHpa.Name,
	}, instance)

	if e != nil {
		log.Errorf("Failed to fetch cronHPA job %s of cronHPA %s in %s namespace,because of %v", job.Name(), cronHpa.Name, cronHpa.Namespace, e)
		return
	}

	deepCopy := instance.DeepCopy()

	var (
		state     autoscalingv1beta1.JobState
		message   string
		eventType string
	)

	err := js.Error
	if err != nil {
		state = autoscalingv1beta1.Failed
		message = fmt.Sprintf("cron hpa failed to execute, because of %v", err)
		eventType = v1.EventTypeWarning
	} else {
		state = autoscalingv1beta1.Succeed
		message = fmt.Sprintf("cron hpa job %s executed successfully. %s", job.name, js.Msg)
		eventType = v1.EventTypeNormal
	}

	condition := autoscalingv1beta1.Condition{
		Name:          job.Name(),
		JobId:         job.ID(),
		RunOnce:       job.RunOnce,
		Schedule:      job.SchedulePlan(),
		TargetSize:    job.DesiredSize,
		LastProbeTime: metav1.Time{Time: time.Now()},
		State:         state,
		Message:       message,
	}

	conditions := instance.Status.Conditions

	var found = false
	for index, c := range conditions {
		if c.JobId == job.ID() || c.Name == job.Name() {
			found = true
			instance.Status.Conditions[index] = condition
		}
	}

	if !found {
		instance.Status.Conditions = append(instance.Status.Conditions, condition)
	}

	err = cm.updateCronHPAStatusWithRetry(instance, deepCopy, job.name)
	if err != nil {
		if _, ok := err.(*NoNeedUpdate); ok {
			log.Warning("No need to update cronHPA, because it is deleted before")
			return
		}
		cm.eventRecorder.Event(instance, v1.EventTypeWarning, "Failed", fmt.Sprintf("Failed to update cronhpa status: %v", err))
	} else {
		cm.eventRecorder.Event(instance, eventType, string(state), message)
	}
}

func (cm *CronManager) updateCronHPAStatusWithRetry(instance *autoscalingv1beta1.CronHorizontalPodAutoscaler, deepCopy *autoscalingv1beta1.CronHorizontalPodAutoscaler, jobName string) error {
	var err error
	if instance == nil {
		log.Warning("Failed to patch cronHPA, because instance is deleted")
		return &NoNeedUpdate{}
	}
	for i := 1; i <= MaxRetryTimes; i++ {
		// leave ResourceVersion = empty
		err = cm.client.Patch(context.Background(), instance, client.MergeFrom(deepCopy))
		if err != nil {
			if errors.IsNotFound(err) {
				log.Error("Failed to patch cronHPA, because instance is deleted")
				return &NoNeedUpdate{}
			}
			log.Errorf("Failed to patch cronHPA %v, because of %v", instance, err)
			continue
		}
		break
	}
	if err != nil {
		log.Errorf("Failed to update cronHPA job %s of cronHPA %s in %s after %d times, because of %v", jobName, instance.Name, instance.Namespace, MaxRetryTimes, err)
	}
	return err
}

func (cm *CronManager) Run(stopChan chan struct{}) {
	cm.cronExecutor.Run()
	cm.gcLoop()
	<-stopChan
	cm.cronExecutor.Stop()
}

// GC loop
func (cm *CronManager) gcLoop() {
	ticker := time.NewTicker(GCInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				log.Infof("GC loop started every %v", GCInterval)
				cm.GC()
			}
		}
	}()
}

// GC will collect all jobs which ref is not exists and recycle.
func (cm *CronManager) GC() {
	log.Infof("Start GC")
	m := make(map[string]CronJob)
	cm.Lock()
	for k, v := range cm.jobQueue {
		m[k] = v
	}
	cm.Unlock()
	current := len(cm.jobQueue)
	log.V(2).Infof("Current active jobs: %d,try to clean up the abandon ones.", current)

	// clean up all metrics
	KubeSubmittedJobsInCronEngineTotal.Set(0)
	KubeSuccessfulJobsInCronEngineTotal.Set(0)
	KubeFailedJobsInCronEngineTotal.Set(0)
	KubeExpiredJobsInCronEngineTotal.Set(0)

	for _, job := range m {
		hpa := job.(*CronJobHPA).HPARef
		found, reason := cm.cronExecutor.FindJob(job)
		if !found {
			if reason == JobTimeOut {
				instance := &autoscalingv1beta1.CronHorizontalPodAutoscaler{}
				if err := cm.client.Get(context.Background(), types.NamespacedName{
					Namespace: hpa.Namespace,
					Name:      hpa.Name,
				}, instance); err != nil {
					log.Errorf("Failed to run time out job %s  due to failed to get cronHPA %s in %s namespace,err: %v", job.Name(), hpa.Name, hpa.Namespace, err)
					continue
				}
				cm.eventRecorder.Event(instance, v1.EventTypeWarning, "OutOfDate", fmt.Sprintf("rerun out of date job: %s", job.Name()))
				if msg, reRunErr := job.Run(); reRunErr != nil {
					log.Errorf("failed to rerun out of date job %s, msg:%s, err %v", job.Name(), msg, reRunErr)
				}
			}

			log.Warningf("Failed to find job %s of cronHPA %s in %s in cron engine and resubmit the job.", job.Name(), hpa.Name, hpa.Namespace)
			cm.cronExecutor.AddJob(job)

			// metrics update
			// when one job is not in cron engine but in crd.
			// That means the job is failed and need to be resubmitted.
			KubeFailedJobsInCronEngineTotal.Add(1)
			KubeSubmittedJobsInCronEngineTotal.Add(1)
			continue
		} else {
			instance := &autoscalingv1beta1.CronHorizontalPodAutoscaler{}
			if err := cm.client.Get(context.Background(), types.NamespacedName{
				Namespace: hpa.Namespace,
				Name:      hpa.Name,
			}, instance); err != nil {
				if errors.IsNotFound(err) {
					log.Infof("remove job %s of cronHPA %s in %s namespace", job.Name(), hpa.Name, hpa.Namespace)
					err := cm.cronExecutor.RemoveJob(job)
					if err != nil {
						log.Errorf("Failed to gc job %s of cronHPA %s in %s namespace", job.Name(), hpa.Name, hpa.Namespace)
						continue
					}
					cm.delete(job.ID())
					// metrics update
					// when a job is in cron engine but not in crd.
					// that means the job has been expired and need to be clean up.
					KubeExpiredJobsInCronEngineTotal.Add(1)
				}

				// metrics update
				// ignore other errors
			}
			conditions := instance.Status.Conditions
			for _, c := range conditions {
				if c.JobId != job.ID() {
					continue
				}
				switch c.State {
				case autoscalingv1beta1.Succeed:
					KubeSuccessfulJobsInCronEngineTotal.Add(1)
				case autoscalingv1beta1.Failed:
					KubeFailedJobsInCronEngineTotal.Add(1)
				case autoscalingv1beta1.Submitted:
					KubeSubmittedJobsInCronEngineTotal.Add(1)
				default:
					KubeSubmittedJobsInCronEngineTotal.Add(1)
				}
			}

		}
	}
	left := len(cm.jobQueue)

	// metrics update
	// set total jobs in cron engine
	KubeJobsInCronEngineTotal.Set(float64(left))

	log.V(2).Infof("Current active jobs: %d, clean up %d jobs.", left, current-left)
}

func NewCronManager(cfg *rest.Config, client client.Client, recorder record.EventRecorder) *CronManager {
	cm := &CronManager{
		cfg:           cfg,
		client:        client,
		jobQueue:      make(map[string]CronJob),
		eventRecorder: recorder,
	}

	hpaClient := clientset.NewForConfigOrDie(cm.cfg)
	discoveryClient := clientset.NewForConfigOrDie(cm.cfg)
	resources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		log.Fatalf("Failed to get api resources, because of %v", err)
	}
	restMapper := restmapper.NewDiscoveryRESTMapper(resources)
	// change the rest mapper to discovery resources
	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(hpaClient.Discovery())
	scaleClient, err := scalelib.NewForConfig(cm.cfg, restMapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)

	if err != nil {
		log.Fatalf("Failed to create scaler client,because of %v", err)
	}

	cm.mapper = restMapper
	cm.scaler = scaleClient

	cm.cronExecutor = NewCronHPAExecutor(nil, cm.JobResultHandler)
	return cm
}
