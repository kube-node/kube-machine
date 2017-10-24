FROM golang:1.9
LABEL maintainer "guus@loodse.com"

RUN git clone https://github.com/kubermatic/machine.git /go/src/github.com/docker/machine  
WORKDIR /go/src/github.com/docker/machine
RUN make build

FROM golang:1.9
LABEL maintainer "guus@loodse.com"

WORKDIR /go/src/github.com/kube-node/kube-machine/
COPY . .
RUN go get -u github.com/golang/dep/cmd/dep
RUN dep ensure -vendor-only
RUN go build -o node-controller cmd/controller/main.go

FROM debian:jessie
LABEL maintainer "henrik@loodse.com"

RUN apt-get update && apt-get install -y ca-certificates curl openssh-server

COPY --from=0 /go/src/github.com/docker/machine/bin/docker-machine /usr/local/bin/docker-machine
RUN chmod +x /usr/local/bin/docker-machine

ADD https://storage.googleapis.com/docker-machine-drivers/docker-machine-driver-otc /usr/local/bin/docker-machine-driver-otc
RUN chmod +x /usr/local/bin/docker-machine-driver-otc

COPY --from=1 /go/src/github.com/kube-node/kube-machine/node-controller /node-controller