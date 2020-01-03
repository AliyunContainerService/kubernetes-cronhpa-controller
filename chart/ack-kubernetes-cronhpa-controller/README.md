## Overview

kubernetes-cronhpa-controller is a kubernetes cron horizontal pod autoscaler controller using crontab like scheme. You can use CronHorizontalPodAutoscaler with any kind object defined in kubernetes which support scale subresource(such as Deployment and StatefulSet).

## Prerequisites

helm version must be v2.11.0+.

## Example

Please try out the examples in the [examples folder](https://github.com/AliyunContainerService/kubernetes-cronhpa-controller/blob/master/examples).

### 1.Deploy sample workload and cronhpa

```txt
$ kubectl apply -f examples/deployment_cronhpa.yaml
```

### 2.Check deployment replicas

```txt
$ kubectl get deploy nginx-deployment-basic
```

If you see what is shown in the figure below, you have successfully deployed workload and cronhpa.

```txt
➜  kubernetes-cronhpa-controller git:(master) ✗ kubectl get deploy nginx-deployment-basic
NAME                     DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
nginx-deployment-basic   2         2         2            2           9s
```

### 3.Describe cronhpa status

```txt
$ kubectl describe cronhpa cronhpa-sample
```

```txt
Name:         cronhpa-sample
Namespace:    default
Labels:       controller-tools.k8s.io=1.0
Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"autoscaling.alibabacloud.com/v1beta1","kind":"CronHorizontalPodAutoscaler","metadata":{"annotations":{},"labels":{"controll...
API Version:  autoscaling.alibabacloud.com/v1beta1
Kind:         CronHorizontalPodAutoscaler
Metadata:
  Creation Timestamp:  2019-04-14T10:42:38Z
  Generation:          1
  Resource Version:    4017247
  Self Link:           /apis/autoscaling.alibabacloud.com/v1beta1/namespaces/default/cronhorizontalpodautoscalers/cronhpa-sample
  UID:                 05e41c95-5ea2-11e9-8ce6-00163e12e274
Spec:
  Jobs:
    Name:         scale-down
    Schedule:     30 */1 * * * *
    Target Size:  1
    Name:         scale-up
    Schedule:     0 */1 * * * *
    Target Size:  3
  Scale Target Ref:
    API Version:  apps/v1beta2
    Kind:         Deployment
    Name:         nginx-deployment-basic
Status:
  Conditions:
    Job Id:           38e79271-9a42-4131-9acd-1f5bfab38802
    Last Probe Time:  2019-04-14T10:43:02Z
    Message:
    Name:             scale-down
    Schedule:         30 */1 * * * *
    State:            Submitted
    Job Id:           a7db95b6-396a-4753-91d5-23c2e73819ac
    Last Probe Time:  2019-04-14T10:43:02Z
    Message:
    Name:             scale-up
    Schedule:         0 */1 * * * *
    State:            Submitted
Events:               <none>
```

if the `State` of cronhpa job is `Succeed` that means the last execution is successful. `Submitted` means the cronhpa job is submitted to the cron engine but haven't be executed so far. Wait for 30s seconds and check the status.

```txt
➜  kubernetes-cronhpa-controller git:(master) kubectl describe cronhpa cronhpa-sample
Name:         cronhpa-sample
Namespace:    default
Labels:       controller-tools.k8s.io=1.0
Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"autoscaling.alibabacloud.com/v1beta1","kind":"CronHorizontalPodAutoscaler","metadata":{"annotations":{},"labels":{"controll...
API Version:  autoscaling.alibabacloud.com/v1beta1
Kind:         CronHorizontalPodAutoscaler
Metadata:
  Creation Timestamp:  2019-04-15T06:41:44Z
  Generation:          1
  Resource Version:    15673230
  Self Link:           /apis/autoscaling.alibabacloud.com/v1beta1/namespaces/default/cronhorizontalpodautoscalers/cronhpa-sample
  UID:                 88ea51e0-5f49-11e9-bd0b-00163e30eb10
Spec:
  Jobs:
    Name:         scale-down
    Schedule:     30 */1 * * * *
    Target Size:  1
    Name:         scale-up
    Schedule:     0 */1 * * * *
    Target Size:  3
  Scale Target Ref:
    API Version:  apps/v1beta2
    Kind:         Deployment
    Name:         nginx-deployment-basic
Status:
  Conditions:
    Job Id:           84818af0-3293-43e8-8ba6-6fd3ad2c35a4
    Last Probe Time:  2019-04-15T06:42:30Z
    Message:          cron hpa job scale-down executed successfully
    Name:             scale-down
    Schedule:         30 */1 * * * *
    State:            Succeed
    Job Id:           f8579f11-b129-4e72-b35f-c0bdd32583b3
    Last Probe Time:  2019-04-15T06:42:20Z
    Message:
    Name:             scale-up
    Schedule:         0 */1 * * * *
    State:            Submitted
Events:
  Type    Reason   Age   From                            Message
  ----    ------   ----  ----                            -------
  Normal  Succeed  5s    cron-horizontal-pod-autoscaler  cron hpa job scale-down executed successfully
```

## configuration

The following table lists the configurable parammeters of the cronhpa ahd their default values.

| Parameter                     | Description                                                  | Default                                                      |
| ----------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| cleanup.cleanupCustomResource | Attempt to delete CRDs when the release is removed. This option may be useful while testing . | true                                                         |
| crds.needcreate               | Whether or not crds is created                               | true                                                         |
| controller.image              | image of the deployment  cronhpa                             | registry.cn-beijing.aliyuncs.com/acs/kubernetes-cronhpa-controller:v1.0 |
| global.rbac.create            | Whether or not rbac is created                               | true                                                         |
