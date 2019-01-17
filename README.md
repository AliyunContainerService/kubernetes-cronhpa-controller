## cron-hpa-controller 
定时伸缩HPA，主要应对周期性负载峰值，可联动cluster-autoscaler实现更好的弹性。

### 架构设计 
设计文档：https://yuque.antfin-inc.com/op0cg2/ekiy8e/uoa2s7

### 快速入门  
1. 安装CRD    

```
kubectl apply -f config/crds/autoscaling_v1beta1_cronhorizontalpodautoscaler.yaml.yaml 
```
2. 部署Controller    

```    
kubectl apply -f config/manager/manager.yaml 
```
3. 部署demo应用      

```
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: nginx-deployment-basic
  labels:
    app: nginx
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.7.9 
        ports:
        - containerPort: 80
```

4. 部署cronHPA     

```
kubectl apply -f config/samples/autoscaling_v1beta1_cronhorizontalpodautoscaler.yaml.yaml 
```

5. 查看状态  

```$xslt
kubectl describe cronhpa cronhorizontalpodautoscaler-sample   
```
可以查看当前的cron
```
Every 1.0s: kubectl describe cronhpa cronhorizontalpodautoscaler-sample                                                                                                 ali-6c96cfdfcc49.local: Thu Jan 17 16:29:03 2019

Name:         cronhorizontalpodautoscaler-sample
Namespace:    default
Labels:       controller-tools.k8s.io=1.0
Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"autoscaling.alibabacloud.com/v1beta1","kind":"CronHorizontalPodAutoscaler","metadata":{"annotations":{},"labels":{"controll...
API Version:  autoscaling.alibabacloud.com/v1beta1
Kind:         CronHorizontalPodAutoscaler
Metadata:
  Creation Timestamp:  2019-01-17T08:21:00Z
  Generation:          1
  Resource Version:    22872543
  Self Link:           /apis/autoscaling.alibabacloud.com/v1beta1/namespaces/default/cronhorizontalpodautoscalers/cronhorizontalpodautoscaler-sample
  UID:                 d2af157d-1a30-11e9-a14f-00163e06b896
Spec:
  Jobs:
    Name:         scale-down
    Schedule:     30 */1 * * * *
    Target Size:  1
  Scale Target Ref:
    API Version:  apps/v1beta2
    Kind:         Deployment
    Name:         nginx-deployment-basic
Status:
  Conditions:
    Job Id:           caaba32a-a2cf-45b5-ab6a-519025edd3d9
    Last Probe Time:  2019-01-17T08:28:30Z
    Message:          cron hpa job scale-down executed successfully
    Name:             scale-down
    Schedule:         30 */1 * * * *
    State:            Succeed
Events:
  Type    Reason   Age                    From                            Message
  ----    ------   ----                   ----                            -------
  Normal  Succeed  7m4s                   cron-horizontal-pod-autoscaler  cron hpa job scale-up executed successfully
  Normal  Succeed  5m34s (x3 over 7m34s)  cron-horizontal-pod-autoscaler  cron hpa job scale-down executed successfully
  Normal  Succeed  34s (x2 over 94s)      cron-horizontal-pod-autoscaler  cron hpa job scale-down executed successfully 
```

### 使用指南
一个标准的`cron-hpa`模板如下，在Spec中支持一个job的数组进行伸缩的配置。
```$xslt
apiVersion: autoscaling.alibabacloud.com/v1beta1
kind: CronHorizontalPodAutoscaler
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: cronhorizontalpodautoscaler-sample
spec:
   scaleTargetRef:
      apiVersion: apps/v1beta2
      kind: Deployment
      name: nginx-deployment-basic
   jobs:
   - name: "scale-down"
     schedule: "30 */1 * * * *"
     targetSize: 1
   - name: "scale-up"
     schedule: "0 */1 * * * *"
     targetSize: 3
```
其中`schedule`字段的定义扩展了Linux的cron任务定义，基于`robfig/cron`引擎。共支持6个字段，具体意义如下：
```$xslt
Field name   | Mandatory? | Allowed values  | Allowed special characters
----------   | ---------- | --------------  | --------------------------
Seconds      | Yes        | 0-59            | * / , -
Minutes      | Yes        | 0-59            | * / , -
Hours        | Yes        | 0-23            | * / , -
Day of month | Yes        | 1-31            | * / , - ?
Month        | Yes        | 1-12 or JAN-DEC | * / , -
Day of week  | Yes        | 0-6 or SUN-SAT  | * / , - ?
```
其中特殊字段含义如下：
```$xslt
Special Characters
Asterisk ( * )

The asterisk indicates that the cron expression will match for all values of the field; e.g., using an asterisk in the 5th field (month) would indicate every month.

Slash ( / )

Slashes are used to describe increments of ranges. For example 3-59/15 in the 1st field (minutes) would indicate the 3rd minute of the hour and every 15 minutes thereafter. The form "*\/..." is equivalent to the form "first-last/...", that is, an increment over the largest possible range of the field. The form "N/..." is accepted as meaning "N-MAX/...", that is, starting at N, use the increment until the end of that specific range. It does not wrap around.

Comma ( , )

Commas are used to separate items of a list. For example, using "MON,WED,FRI" in the 5th field (day of week) would mean Mondays, Wednesdays and Fridays.

Hyphen ( - )

Hyphens are used to define ranges. For example, 9-17 would indicate every hour between 9am and 5pm inclusive.

Question mark ( ? )

Question mark may be used instead of '*' for leaving either day-of-month or day-of-week blank.

```
典型的`schedule`设置如下：
```$xslt
每天早上8点：0 0 8 * * * 
每小时第5分钟：0 5 * * * 
每隔5分钟: 0 */5 * * * 
``` 
更多时间设置参考<a href="https://godoc.org/github.com/robfig/cron" target="_blank">GoDoc</a>。此处需要重点注意是，在kubernetes中，建议最小的时间间隔不要低于一分钟，因为低于1分钟可能会导致组件的更新导致异常与震荡。

### 常见问题
1. 时区问题
默认`cron-hpa-controller`会采用当前所设置的时区，所以如果需要设置和时区有关的`cron-hpa`的话，需要重点关注时区的问题。    

2. 出现BUG如何快速止血    
对于定时任务类型的controller而言，一旦出现问题由于任务引擎的复杂性，可能会很难排查和恢复。在`cron-hpa-controller`中，一旦出现异常情况，可以先kill掉Pod，此时重新拉起的controller会尝试进行自愈，如果问题依然无法解决，可以先删除相关的cronhpa，再重启controller。最后别忘了提交issues到社区得到最快速的反馈。

### 贡献代码  
cron-hpa-controller是基于kube-builder的框架进行生成的，如果需要增加subresource，请参照kube-builder的规范。
1. 测试代码     

```
make test 
```
2. 构建镜像   

```$xslt
make docker-build IMG=[your_image:tag]
```