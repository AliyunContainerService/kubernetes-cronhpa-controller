/*
Copyright 2018 The Kubernetes Authors.

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

package apiserver

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	unavailableCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aggregator_unavailable_apiservice_count",
			Help: "Counter of APIServices which are marked as unavailable broken down by APIService name and reason.",
		},
		[]string{"name", "reason"},
	)
	unavailableGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aggregator_unavailable_apiservice",
			Help: "Gauge of APIServices which are marked as unavailable broken down by APIService name.",
		},
		[]string{"name"},
	)
)

func init() {
	prometheus.MustRegister(unavailableCounter)
	prometheus.MustRegister(unavailableGauge)
}
