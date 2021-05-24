package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	"github.com/ringtail/go-cron"
	"github.com/satori/go.uuid"
	autoscalingapi "k8s.io/api/autoscaling/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	scaleclient "k8s.io/client-go/scale"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

const (
	updateRetryInterval = 3 * time.Second
	maxRetryTimeout     = 10 * time.Second
	dateFormat          = "11-15-1990"
)

type CronJob interface {
	ID() string
	Name() string
	SetID(id string)
	Equals(Job CronJob) bool
	SchedulePlan() string
	Ref() *TargetRef
	CronHPAMeta() *v1beta1.CronHorizontalPodAutoscaler
	Run() (msg string, err error)
}

type TargetRef struct {
	RefName      string
	RefNamespace string
	RefKind      string
	RefGroup     string
	RefVersion   string
}

// needed when compare equals.
func (tr *TargetRef) toString() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", tr.RefName, tr.RefNamespace, tr.RefKind, tr.RefGroup, tr.RefVersion)
}

type CronJobHPA struct {
	TargetRef    *TargetRef
	HPARef       *v1beta1.CronHorizontalPodAutoscaler
	id           string
	name         string
	DesiredSize  int32
	Plan         string
	RunOnce      bool
	scaler       scaleclient.ScalesGetter
	mapper       apimeta.RESTMapper
	excludeDates []string
	client       client.Client
}

func (ch *CronJobHPA) SetID(id string) {
	ch.id = id
}

func (ch *CronJobHPA) Name() string {
	return ch.name
}

func (ch *CronJobHPA) ID() string {
	return ch.id
}

func (ch *CronJobHPA) Equals(j CronJob) bool {
	// update will create a new uuid
	if ch.id == j.ID() && ch.SchedulePlan() == j.SchedulePlan() && ch.Ref().toString() == j.Ref().toString() {
		return true
	}
	return false
}

func (ch *CronJobHPA) SchedulePlan() string {
	return ch.Plan
}

func (ch *CronJobHPA) Ref() *TargetRef {
	return ch.TargetRef
}

func (ch *CronJobHPA) CronHPAMeta() *v1beta1.CronHorizontalPodAutoscaler {
	return ch.HPARef
}

func (ch *CronJobHPA) Run() (msg string, err error) {

	if skip, msg := IsTodayOff(ch.excludeDates); skip {
		return msg, nil
	}

	startTime := time.Now()
	times := 0
	for {
		now := time.Now()

		// timeout and exit
		if startTime.Add(maxRetryTimeout).Before(now) {
			return "", fmt.Errorf("failed to scale %s %s in %s namespace to %d after retrying %d times and exit,because of %v", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace, ch.DesiredSize, times, err)
		}

		// hpa compatible
		if ch.TargetRef.RefKind == "HorizontalPodAutoscaler" {
			msg, err = ch.ScaleHPA()
			if err == nil {
				break
			}
		} else {
			msg, err = ch.ScalePlainRef()
			if err == nil {
				break
			}
		}
		time.Sleep(updateRetryInterval)
		times = times + 1
	}

	return msg, err
}

func (ch *CronJobHPA) ScaleHPA() (msg string, err error) {
	var scale *autoscalingapi.Scale
	var targetGR schema.GroupResource

	ctx := context.Background()
	hpa := &autoscalingapi.HorizontalPodAutoscaler{}
	err = ch.client.Get(ctx, types.NamespacedName{Namespace: ch.HPARef.Namespace, Name: ch.TargetRef.RefName}, hpa)

	if err != nil {
		return "", fmt.Errorf("Failed to get HorizontalPodAutoscaler Ref,because of %v", err)
	}

	targetRef := hpa.Spec.ScaleTargetRef

	targetGV, err := schema.ParseGroupVersion(targetRef.APIVersion)
	if err != nil {
		return "", fmt.Errorf("Failed to get TargetGroup of HPA %s,because of %v", hpa.Name, err)
	}

	targetGK := schema.GroupKind{
		Kind:  targetRef.Kind,
		Group: targetGV.Group,
	}

	mappings, err := ch.mapper.RESTMappings(targetGK)
	if err != nil {
		return "", fmt.Errorf("Failed to create mapping,because of %v", err)
	}

	found := false
	for _, mapping := range mappings {
		targetGR = mapping.Resource.GroupResource()
		scale, err = ch.scaler.Scales(ch.TargetRef.RefNamespace).Get(context.Background(), targetGR, targetRef.Name, v1.GetOptions{})
		if err == nil {
			found = true
			break
		}
	}

	if found == false {
		log.Errorf("failed to found source target %s %s in %s namespace", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace)
		return "", fmt.Errorf("failed to found source target %s %s in %s namespace", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace)
	}

	updateHPA := false

	if ch.DesiredSize > hpa.Spec.MaxReplicas {
		hpa.Spec.MaxReplicas = ch.DesiredSize
		updateHPA = true
	}

	if ch.DesiredSize < *hpa.Spec.MinReplicas {
		*hpa.Spec.MinReplicas = ch.DesiredSize
		updateHPA = true
	}

	//
	if hpa.Status.CurrentReplicas == *hpa.Spec.MinReplicas && ch.DesiredSize < hpa.Status.CurrentReplicas {
		*hpa.Spec.MinReplicas = ch.DesiredSize
		updateHPA = true
	}

	if hpa.Status.CurrentReplicas < ch.DesiredSize {
		*hpa.Spec.MinReplicas = ch.DesiredSize
		updateHPA = true
	}

	if updateHPA {
		err = ch.client.Update(ctx, hpa)
		if err != nil {
			return "", err
		}
	}

	if hpa.Status.CurrentReplicas >= ch.DesiredSize {
		// skip change replicas and exit
		return fmt.Sprintf("Skip scale replicas because HPA %s current replicas:%d >= desired replicas:%d.", hpa.Name, scale.Spec.Replicas, ch.DesiredSize), nil
	}

	msg = fmt.Sprintf("current replicas:%d, desired replicas:%d.", scale.Spec.Replicas, ch.DesiredSize)

	scale.Spec.Replicas = int32(ch.DesiredSize)
	_, err = ch.scaler.Scales(ch.TargetRef.RefNamespace).Update(context.Background(), targetGR, scale, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to scale %s %s in %s namespace to %d, because of %v", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace, ch.DesiredSize, err)
	}
	return msg, nil
}

func (ch *CronJobHPA) ScalePlainRef() (msg string, err error) {
	var scale *autoscalingapi.Scale
	var targetGR schema.GroupResource

	targetGK := schema.GroupKind{
		Group: ch.TargetRef.RefGroup,
		Kind:  ch.TargetRef.RefKind,
	}
	mappings, err := ch.mapper.RESTMappings(targetGK)
	if err != nil {
		return "", fmt.Errorf("Failed to create create mapping,because of %v", err)
	}

	found := false
	for _, mapping := range mappings {
		targetGR = mapping.Resource.GroupResource()
		scale, err = ch.scaler.Scales(ch.TargetRef.RefNamespace).Get(context.Background(), targetGR, ch.TargetRef.RefName, v1.GetOptions{})
		if err == nil {
			found = true
			log.Infof("%s %s in namespace %s has been scaled successfully. job: %s replicas: %d", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace, ch.Name(), ch.DesiredSize)
			break
		}
	}

	if found == false {
		log.Errorf("failed to find source target %s %s in %s namespace", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace)
		return "", fmt.Errorf("failed to find source target %s %s in %s namespace", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace)
	}

	msg = fmt.Sprintf("current replicas:%d, desired replicas:%d.", scale.Spec.Replicas, ch.DesiredSize)

	scale.Spec.Replicas = int32(ch.DesiredSize)
	_, err = ch.scaler.Scales(ch.TargetRef.RefNamespace).Update(context.Background(), targetGR, scale, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to scale %s %s in %s namespace to %d, because of %v", ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.TargetRef.RefNamespace, ch.DesiredSize, err)
	}
	return msg, nil
}

func checkRefValid(ref *TargetRef) error {
	if ref.RefVersion == "" || ref.RefGroup == "" || ref.RefName == "" || ref.RefNamespace == "" || ref.RefKind == "" {
		return errors.New("any properties in ref could not be empty")
	}
	return nil
}

func checkPlanValid(plan string) error {
	return nil
}

func CronHPAJobFactory(instance *v1beta1.CronHorizontalPodAutoscaler, job v1beta1.Job, scaler scaleclient.ScalesGetter, mapper apimeta.RESTMapper, client client.Client) (CronJob, error) {
	arr := strings.Split(instance.Spec.ScaleTargetRef.ApiVersion, "/")
	group := arr[0]
	version := arr[1]
	ref := &TargetRef{
		RefName:      instance.Spec.ScaleTargetRef.Name,
		RefKind:      instance.Spec.ScaleTargetRef.Kind,
		RefNamespace: instance.Namespace,
		RefGroup:     group,
		RefVersion:   version,
	}

	if err := checkRefValid(ref); err != nil {
		return nil, err
	}
	if err := checkPlanValid(job.Schedule); err != nil {
		return nil, err
	}
	return &CronJobHPA{
		id:           uuid.Must(uuid.NewV4(), nil).String(),
		TargetRef:    ref,
		HPARef:       instance,
		name:         job.Name,
		Plan:         job.Schedule,
		DesiredSize:  job.TargetSize,
		RunOnce:      job.RunOnce,
		scaler:       scaler,
		mapper:       mapper,
		excludeDates: instance.Spec.ExcludeDates,
		client:       client,
	}, nil
}

func IsTodayOff(excludeDates []string) (bool, string) {

	if excludeDates == nil {
		return false, ""
	}

	now := time.Now()
	for _, date := range excludeDates {
		schedule, err := cron.Parse(date)
		if err != nil {
			log.Warningf("Failed to parse schedule %s,and skip this date,because of %v", date, err)
			continue
		}
		if nextTime := schedule.Next(now); nextTime.Format(dateFormat) == now.Format(dateFormat) {
			return true, fmt.Sprintf("skip scaling activity,because of excludeDate (%s).", date)
		}
	}
	return false, ""
}
