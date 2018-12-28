package cronhorizontalpodautoscaler

import (
	scaleclient "k8s.io/client-go/scale"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"github.com/satori/go.uuid"
	autoscalingapi "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"github.com/virtual-kubelet/virtual-kubelet/providers/aliyun/ingress/errors"
	"fmt"
	"gitlab.alibaba-inc.com/cos/kubernetes-cron-hpa-controller/pkg/apis/autoscaling/v1beta1"
)

type CronJob interface {
	ID() string
	Name() string
	SetID(id string)
	Equals(Job CronJob) bool
	SchedulePlan() string
	Run() error
}

type TargetRef struct {
	RefName      string
	RefNamespace string
	RefKind      string
	RefGroup     string
	RefVersion   string
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
	if ch.id == j.ID() && ch.SchedulePlan() == j.SchedulePlan() {
		return true
	}
	return false
}

func (ch *CronJobHPA) SchedulePlan() string {
	return ch.Plan
}

func (ch *CronJobHPA) Run() error {
	targetGK := schema.GroupKind{
		Group: ch.TargetRef.RefGroup,
		Kind:  ch.TargetRef.RefKind,
	}
	mappings, err := ch.mapper.RESTMappings(targetGK)
	if err != nil {
		return fmt.Errorf("Failed to create create mapping,because of %s", err.Error())
	}

	var scale *autoscalingapi.Scale
	var targetGR schema.GroupResource
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
		return fmt.Errorf("Failed to found source target %s", ch.TargetRef.RefName)
	}
	scale.Spec.Replicas = int32(ch.DesiredSize)
	_, err = ch.scaler.Scales(ch.TargetRef.RefNamespace).Update(targetGR, scale)
	if err != nil {
		return fmt.Errorf("Failed to scale (namespace: %s;kind: %s;name: %s) to %d,because of %s", ch.TargetRef.RefNamespace, ch.TargetRef.RefKind, ch.TargetRef.RefName, ch.DesiredSize, err.Error())
	}
	return nil
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

func CronHPAJobFactory(ref *TargetRef, hpaRef *v1beta1.CronHorizontalPodAutoscaler, name string, plan string, size int32, scaler scaleclient.ScalesGetter, mapper apimeta.RESTMapper) (CronJob, error) {
	if err := checkRefValid(ref); err != nil {
		return nil, err
	}
	if err := checkPlanValid(plan); err != nil {
		return nil, err
	}
	return &CronJobHPA{
		id:          uuid.Must(uuid.NewV4()).String(),
		TargetRef:   ref,
		name:        name,
		HPARef:      hpaRef,
		Plan:        plan,
		DesiredSize: size,
		scaler:      scaler,
		mapper:      mapper,
	}, nil
}
