#!/usr/bin/env bash
export GOPATH=/Users/ringtail/go
vendor/k8s.io/code-generator/generate-groups.sh all \
gitlab.alibaba-inc.com/cos/kubernetes-cron-hpa-controller/pkg/client \ gitlab.alibaba-inc.com/cos/kubernetes-cron-hpa-controller/pkg/apis \autoscaling:v1beta1