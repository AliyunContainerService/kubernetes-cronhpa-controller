package cronhorizontalpodautoscaler

import (
	"context"
	"fmt"
	autoscalingv1beta1 "github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	"github.com/ringtail/go-cron"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"
)

const (
	MaxRetryTimes    = 3
	MaxRetryInterval = 1 * time.Second
)

type NoNeedUpdate struct{}

func (n NoNeedUpdate) Error() string {
	return "NoNeedUpate"
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
	} else {
		job := cm.jobQueue[j.ID()]
		if ok := job.Equals(j); !ok {
			err := cm.cronExecutor.Update(j)
			if err != nil {
				return fmt.Errorf("Failed to update job to cronExecutor,because of %v", err)
			}
			//update job queue
			cm.jobQueue[j.ID()] = j
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

	deepCopy := instance.DeepCopy()
	if e != nil {
		log.Errorf("Failed to fetch CronHorizontalPodAutoscaler job:%s namespace:%s cronHPA:%s,because of %v", job.Name(), cronHpa.Namespace, cronHpa.Name, e)
		return
	}

	var (
		state     autoscalingv1beta1.JobState
		message   string
		eventType string
	)

	err := js.Error
	if err != nil {
		state = autoscalingv1beta1.Failed
		message = fmt.Sprintf("cron hpa failed to execute,because of %v", err)
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
		cm.eventRecorder.Event(instance, v1.EventTypeWarning, "Failed", fmt.Sprintf("Failed to update cronhpa status: %v", err))
	} else {
		cm.eventRecorder.Event(instance, eventType, string(state), message)
	}
}

func (cm *CronManager) updateCronHPAStatusWithRetry(instance *autoscalingv1beta1.CronHorizontalPodAutoscaler, deepCopy *autoscalingv1beta1.CronHorizontalPodAutoscaler, jobName string) error {
	var err error
	for i := 1; i <= MaxRetryTimes; i++ {
		// leave ResourceVersion = empty
		err = cm.client.Patch(context.Background(), instance, client.MergeFrom(deepCopy))
		if err != nil {
			log.Errorf("Failed to patch cronHPAJob %v,because of %v", instance, err)
			continue
		}
		break
	}
	if err != nil {
		log.Errorf("Failed to update cronHPA job:%s namespace:%s cronHPA:%s after %d times,because of %v", jobName, instance.Namespace, instance.Name, MaxRetryTimes, err)
	}
	return err
}

func (cm *CronManager) Run(stopChan chan struct{}) {
	cm.cronExecutor.Run()
	<-stopChan
	cm.cronExecutor.Stop()
}

// GC will collect all jobs which ref is not exists and recycle.
func (cm *CronManager) GC() {
	m := make(map[string]CronJob)
	cm.Lock()
	for k, v := range cm.jobQueue {
		m[k] = v
	}
	cm.Unlock()

	for _, job := range m {
		hpa := job.(*CronJobHPA).HPARef
		instance := &autoscalingv1beta1.CronHorizontalPodAutoscaler{}
		if err := cm.client.Get(context.Background(), types.NamespacedName{
			Namespace: hpa.Namespace,
			Name:      hpa.Name,
		}, instance); err != nil {
			if errors.IsNotFound(err) {
				err := cm.cronExecutor.RemoveJob(job)
				if err != nil {
					log.Errorf("Failed to gc job %s %s %s", hpa.Namespace, hpa.Name, job.Name())
					continue
				}
				cm.delete(job.ID())
			}
		}
	}
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
	cachedClient := cacheddiscovery.NewMemCacheClient(discoveryClient.Discovery())
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedClient)

	go wait.Until(func() {
		restMapper.Reset()
	}, 30*time.Second, wait.NeverStop)

	// change the rest mapper to discovery resources
	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(hpaClient.Discovery())
	scaleClient, err := scale.NewForConfig(cm.cfg, restMapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)

	if err != nil {
		log.Fatalf("Failed to create scaler client,because of %v", err)
	}

	cm.mapper = restMapper
	cm.scaler = scaleClient

	cm.cronExecutor = NewCronHPAExecutor(nil, cm.JobResultHandler)
	return cm
}
