package cronhorizontalpodautoscaler

import (
	"errors"
	"fmt"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	log "github.com/Sirupsen/logrus"
	"github.com/satori/go.uuid"
	autoscalingapi "k8s.io/api/autoscaling/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	scaleclient "k8s.io/client-go/scale"
	"time"
)

const (
	updateRetryInterval = 5 * time.Second
	maxRetryTimeout     = 1 * time.Minute
)

type CronJob interface {
	ID() string
	Name() string
	SetID(id string)
	Equals(Job CronJob) bool
	SchedulePlan() string
	Ref() *TargetRef
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
	TargetRef   *TargetRef
	HPARef      *v1beta1.CronHorizontalPodAutoscaler
	id          string
	name        string
	DesiredSize int32
	Plan        string

	scaler scaleclient.ScalesGetter
	mapper apimeta.RESTMapper
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

func (ch *CronJobHPA) Run() (msg string, err error) {
	targetGK := schema.GroupKind{
		Group: ch.TargetRef.RefGroup,
		Kind:  ch.TargetRef.RefKind,
	}
	mappings, err := ch.mapper.RESTMappings(targetGK)
	if err != nil {
		return "", fmt.Errorf("Failed to create create mapping,because of %s", err.Error())
	}

	var scale *autoscalingapi.Scale
	var targetGR schema.GroupResource

	startTime := time.Now()
	times := 0

	for {
		now := time.Now()

		// timeout and exit
		if startTime.Add(maxRetryTimeout).Before(now) {
			return "", fmt.Errorf("Failed to scale (namespace: %s;kind: %s;name: %s) to %d after retrying %d times and exit,because of %s", ch.TargetRef.RefNamespace, ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.DesiredSize, times, err.Error())
		}

		found := false
		for _, mapping := range mappings {
			targetGR = mapping.Resource.GroupResource()
			scale, err = ch.scaler.Scales(ch.TargetRef.RefNamespace).Get(targetGR, ch.TargetRef.RefName)
			if err == nil {
				found = true
				break
			}
		}

		if found == false {
			return "", fmt.Errorf("Failed to found source target %s", ch.TargetRef.RefName)
		}

		msg = fmt.Sprintf("current replicas:%d, desired replicas:%d", scale.Spec.Replicas, ch.DesiredSize)
		scale.Spec.Replicas = int32(ch.DesiredSize)
		_, err = ch.scaler.Scales(ch.TargetRef.RefNamespace).Update(targetGR, scale)
		if err != nil {
			log.Warningf("Failed to scale (namespace: %s;kind: %s;name: %s) to %d,because of %s", ch.TargetRef.RefNamespace, ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.DesiredSize, err.Error())
		} else {
			break
		}

		time.Sleep(updateRetryInterval)
		times = times + 1
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

func CronHPAJobFactory(ref *TargetRef, hpaRef *v1beta1.CronHorizontalPodAutoscaler, job v1beta1.Job, scaler scaleclient.ScalesGetter, mapper apimeta.RESTMapper) (CronJob, error) {
	if err := checkRefValid(ref); err != nil {
		return nil, err
	}
	if err := checkPlanValid(job.Schedule); err != nil {
		return nil, err
	}
	return &CronJobHPA{
		id:          uuid.Must(uuid.NewV4(), nil).String(),
		TargetRef:   ref,
		HPARef:      hpaRef,
		name:        job.Name,
		Plan:        job.Schedule,
		DesiredSize: job.TargetSize,
		scaler:      scaler,
		mapper:      mapper,
	}, nil
}
