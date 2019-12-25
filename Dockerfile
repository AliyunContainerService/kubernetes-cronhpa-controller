# Build the manager binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/AliyunContainerService/kubernetes-cronhpa-controller
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o kubernetes-cronhpa-controller github.com/AliyunContainerService/kubernetes-cronhpa-controller/cmd/kubernetes-cronhpa-controller

# Copy the controller-manager into a thin image
FROM alpine:3.10
RUN apk add --no-cache tzdata
WORKDIR /root/
COPY --from=builder /go/src/github.com/AliyunContainerService/kubernetes-cronhpa-controller/kubernetes-cronhpa-controller .
COPY docker-entrypoint.sh .
ENTRYPOINT  ["docker-entrypoint.sh"]
CMD ["/root/kubernetes-cronhpa-controller"]
