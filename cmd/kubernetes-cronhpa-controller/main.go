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

package main

import (
	"flag"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis"
	autoscalingv1beta1 "github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis/autoscaling/v1beta1"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/controller"
	klog "k8s.io/klog/v2"
	"net/http"
	_ "net/http/pprof"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var (
	enableLeaderElection bool
	pprofAddr            string
	metricsAddr          string
)

func main() {
	flag.StringVar(&pprofAddr, "pprof-bind-address", ":6060", "The address the pprof endpoint binds to.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.Parse()
	klog.Info("Start cronHPA controller.")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "kubernetes-cronhpa-controller",
		MetricsBindAddress: metricsAddr,
	})
	if err != nil {
		klog.Errorf("Failed to set up controller manager,because of %v", err)
		os.Exit(1)
	}

	// in a real controller, we'd create a new scheme for this
	err = apis.AddToScheme(mgr.GetScheme())
	if err != nil {
		klog.Errorf("Failed to add apis to scheme,because of %v", err)
		os.Exit(1)
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1beta1.CronHorizontalPodAutoscaler{}).
		Complete(controller.NewReconciler(mgr))
	if err != nil {
		klog.Errorf("Failed to set up controller watch loop,because of %v", err)
		os.Exit(1)
	}

	go func() {
		http.ListenAndServe(pprofAddr, nil)
	}()

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		klog.Errorf("Failed to start up controller manager,because of %v", err)
		os.Exit(1)
	}
}

func init() {
	flag.BoolVar(&enableLeaderElection, "enableLeaderElection", false, "default false, if enabled the cronHPA would be in primary and standby mode.")
	klog.InitFlags(nil)
}
