FROM golang:1.9
LABEL maintainer "guus@loodse.com"

RUN git clone https://github.com/kubermatic/machine.git /go/src/github.com/docker/machine  
WORKDIR /go/src/github.com/docker/machine
RUN make build

FROM debian:jessie
LABEL maintainer "henrik@loodse.com"

RUN apt-get update && apt-get install -y ca-certificates curl openssh-server

ADD _output/docker-machine /usr/local/bin/docker-machine
RUN chmod +x /usr/local/bin/docker-machine

ADD https://storage.googleapis.com/docker-machine-drivers/docker-machine-driver-otc /usr/local/bin/docker-machine-driver-otc
RUN chmod +x /usr/local/bin/docker-machine-driver-otc

ADD _output/node-controller /node-controller		