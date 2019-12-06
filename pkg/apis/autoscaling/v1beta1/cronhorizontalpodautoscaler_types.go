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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/ringtail/go-cron"
	"github.com/asaskevich/govalidator"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CronHorizontalPodAutoscalerSpec defines the desired state of CronHorizontalPodAutoscaler
type CronHorizontalPodAutoscalerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ExcludeDates   			[]string       `json:"excludeDates" valid:"excludeDatesValidator"`
	ExcludeDatesConfigMap   string         `json:"excludeDatesConfigMap"`
	ScaleTargetRef 			ScaleTargetRef `json:"scaleTargetRef"`
	Jobs           			[]Job          `json:"jobs"`
}

type Job struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	// job will only run once if enabled.
	RunOnce    bool  `json:"runOnce"`
	TargetSize int32 `json:"targetSize"`
}

type ScaleTargetRef struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

type JobState string

const (
	Succeed   JobState = "Succeed"
	Failed    JobState = "Failed"
	Submitted JobState = "Submitted"
)

type Condition struct {
	// Type of job condition, Complete or Failed.
	Name string `json:"name"`

	JobId string `json:"jobId"`

	Schedule string `json:"schedule"`

	TargetSize int32 `json:"targetSize"`

	RunOnce bool `json:"runOnce"`

	State JobState `json:"state"`

	LastProbeTime metav1.Time `json:"lastProbeTime"`

	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message"`
}

// CronHorizontalPodAutoscalerStatus defines the observed state of CronHorizontalPodAutoscaler
type CronHorizontalPodAutoscalerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	ScaleTargetRef ScaleTargetRef `json:"scaleTargetRef"`
	ExcludeDates   []string       `json:"excludeDates"`
	ExcludeDatesConfigMap   string         `json:"excludeDatesConfigMap"`
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []Condition `json:"conditions"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CronHorizontalPodAutoscaler is the Schema for the cronhorizontalpodautoscalers API
// +k8s:openapi-gen=true
type CronHorizontalPodAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CronHorizontalPodAutoscalerSpec   `json:"spec,omitempty"`
	Status CronHorizontalPodAutoscalerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CronHorizontalPodAutoscalerList contains a list of CronHorizontalPodAutoscaler
type CronHorizontalPodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronHorizontalPodAutoscaler `json:"items"`
}

func init() {
	govalidator.CustomTypeTagMap.Set("excludeDatesValidator", func(i interface{}, context interface{}) bool {
		switch v := i.(type) {
		case []string:
			parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			for _, item := range v {
				_, err := parser.Parse(item)
				if err != nil {
					return false
				}
			}
			return true
		}
		return false
	})

	SchemeBuilder.Register(&CronHorizontalPodAutoscaler{}, &CronHorizontalPodAutoscalerList{})
}
