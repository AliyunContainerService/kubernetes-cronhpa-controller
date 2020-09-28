#!/usr/bin/env bash

ROOT_PACKAGE="github.com/AliyunContainerService/kubernetes-cronhpa-controller"
GO111MODULE=off
# 安装k8s.io/code-generator
[[ -d $GOPATH/src/k8s.io/code-generator ]] || go get -u k8s.io/code-generator/...

$GOPATH/src/k8s.io/code-generator/generate-groups.sh all "${ROOT_PACKAGE}/pkg/client" "${ROOT_PACKAGE}/pkg/apis" "autoscaling:v1beta1"