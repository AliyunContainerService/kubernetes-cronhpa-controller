#!/usr/bin/env bash
export GOPATH=/Users/ringtail/go
vendor/k8s.io/code-generator/generate-groups.sh all \
github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/client \ github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/apis \autoscaling:v1beta1