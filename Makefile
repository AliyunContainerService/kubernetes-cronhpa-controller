# build params
PREFIX?=registry.aliyuncs.com/acs
VERSION?=v1.4.0
GIT_COMMIT:=$(shell git rev-parse --short HEAD)
CRD_OPTIONS ?= "crd:trivialVersions=true,maxDescLen=0"

# Image URL to use all building/pushing image targets
IMG ?= $(PREFIX)/kubernetes-cronhpa-controller:$(VERSION)-$(GIT_COMMIT)-aliyun

.PHONY: all
all: test kubernetes-cronhpa-controller

.PHONY: test
# Run tests
test: generate fmt vet
	TRAVIS=true go test github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/... \
	    github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/...  \
	        -coverprofile cover.out

.PHONY: kubernetes-cronhpa-controller
# Build kubernetes-cronhpa-controller binary
kubernetes-cronhpa-controller: generate fmt vet
	go build -o bin/kubernetes-cronhpa-controller github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/kubernetes-cronhpa-controller

.PHONY: run
# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/kubernetes-cronhpa-controller/main.go

.PHONY: install
# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

.PHONY: deploy
# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

.PHONY: manifests
# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=config/crds

.PHONY: fmt
# Run go fmt against code
fmt:
	go fmt github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/... \
           	    github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/...

.PHONY: vet
# Run go vet against code
vet:
	go vet github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/... \
           	    github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/...

.PHONY: generate
# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: docker-build
# Build the docker image
docker-build: test
	pod build . -t ${IMG}
	@echo "updating kustomize image patch file for kubernetes-cronhpa-controller resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

.PHONY: docker-push
# Push the docker image
docker-push:
	docker push ${IMG}


.PHONY: controller-gen
controller-gen: 
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif