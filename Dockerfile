FROM debian:jessie
LABEL maintainer "henrik@loodse.com"

RUN apt-get update && apt-get install -y ca-certificates curl openssh-server

ADD binaries/_output-docker-machine/docker-machine /usr/local/bin/docker-machine
RUN chmod +x /usr/local/bin/docker-machine

ADD binaries/_output-kube-machine/node-controller /node-controller
RUN chmod +x /node-controller
