package cronhorizontalpodautoscaler

import (
	"context"
	"fmt"
	autoscalingv1beta1 "github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	log "github.com/Sirupsen/logrus"
	"github.com/ringtail/go-cron"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"
)

type NoNeedUpdate struct{}

func (n NoNeedUpdate) Error() string {
	return "NoNeedUpate"
}

type CronManager struct {
	sync.Mutex
	cfg           *rest.Config
	client        client.Client
	jobQueue      map[string]CronJob
	cronProcessor CronProcessor
	cronExecutor  CronExecutor
	mapper        *restmapper.DeferredDiscoveryRESTMapper
	scaler        scale.ScalesGetter
	eventRecorder record.EventRecorder
}

func (cm *CronManager) createOrUpdate(j CronJob) error {
	cm.Lock()
	defer cm.Unlock()
	if _, ok := cm.jobQueue[j.ID()]; !ok {
		err := cm.cronExecutor.AddJob(j)
		if err != nil {
			return fmt.Errorf("Failed to add job to cronExecutor,because of %s", err.Error())
		}
		cm.jobQueue[j.ID()] = j
	} else {
		job := cm.jobQueue[j.ID()]
		if ok := job.Equals(j); !ok {
			err := cm.cronExecutor.Update(j)
			if err != nil {
				return fmt.Errorf("Failed to update job to cronExecutor,because of %s", err.Error())
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
			return fmt.Errorf("Failed to remove job from cronExecutor,because of %s", err.Error())
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

	if e != nil {
		log.Errorf("Failed to fetch CronHorizontalPodAutoscaler job:%s namespace:%s cronHPA:%s,because of %s", job.Name(), cronHpa.Namespace, cronHpa.Name, e.Error())
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
		message = fmt.Sprintf("cron hpa failed to execute,because of %s", err.Error())
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

	if found == false {
		instance.Status.Conditions = append(instance.Status.Conditions, condition)
	}

	err = cm.client.Update(context.Background(), instance)
	if err != nil {
		log.Errorf("Failed to update cronHPA job:%s namespace:%s cronHPA:%s,because of %s", job.Name(), cronHpa.Namespace, cronHpa.Name, err.Error())
	}
	cm.eventRecorder.Event(instance, eventType, string(state), message)
}

func (cm *CronManager) Run(stopChan chan struct{}) {
	rootClientBuilder := controller.SimpleControllerClientBuilder{
		ClientConfig: cm.cfg,
	}

	versionedClient := rootClientBuilder.ClientOrDie("shared-informers")
	sharedInformers := informers.NewSharedInformerFactory(versionedClient, time.Minute*5)

	// Use a discovery client capable of being refreshed.
	discoveryClient := rootClientBuilder.ClientOrDie("controller-discovery")
	cachedClient := cacheddiscovery.NewMemCacheClient(discoveryClient.Discovery())
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedClient)

	go wait.Until(func() {
		restMapper.Reset()
	}, 30*time.Second, stopChan)

	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(rootClientBuilder.ClientOrDie("horizontal-pod-autoscaler").Discovery())
	scaler, err := scale.NewForConfig(rootClientBuilder.ConfigOrDie("horizontal-pod-autoscaler"), restMapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	if err != nil {
		log.Fatal(err)
	}
	cm.mapper = restMapper
	cm.scaler = scaler

	sharedInformers.Start(stopChan)

	cm.cronExecutor.Run()
	<-stopChan
	cm.cronExecutor.Stop()
}

// GC will collect all jobs which ref is not exists and recycle.
func (cm *CronManager) GC() {
	for _, job := range cm.jobQueue {
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
				delete(cm.jobQueue, job.ID())
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
	cm.cronExecutor = NewCronHPAExecutor(nil, cm.JobResultHandler)
	return cm
}
