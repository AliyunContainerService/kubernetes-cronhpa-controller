# build params
PREFIX?=registry.aliyuncs.com/acs
VERSION?=v1.4.0
GIT_COMMIT:=$(shell git rev-parse --short HEAD)
CRD_OPTIONS ?= "crd:trivialVersions=true,maxDescLen=0"

# Image URL to use all building/pushing image targets
IMG ?= $(PREFIX)/kubernetes-cronhpa-controller:$(VERSION)-$(GIT_COMMIT)-aliyun
all: test kubernetes-cronhpa-controller

# Run tests
test: generate fmt vet
	TRAVIS=true go test github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/... \
	    github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/...  \
	        -coverprofile cover.out

# Build kubernetes-cronhpa-controller binary
kubernetes-cronhpa-controller: generate fmt vet
	go build -o bin/kubernetes-cronhpa-controller github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/kubernetes-cronhpa-controller

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/kubernetes-cronhpa-controller/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=config/crds

# Run go fmt against code
fmt:
	go fmt github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/... \
           	    github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/...

# Run go vet against code
vet:
	go vet github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/... \
           	    github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/...

# Generate code
generate: controller-gen
    $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: test
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for kubernetes-cronhpa-controller resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
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