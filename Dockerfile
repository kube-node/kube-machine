FROM debian:jessie
LABEL maintainer "henrik@loodse.com"

RUN apt-get update && apt-get install -y ca-certificates curl openssh-server

RUN curl -o /usr/local/bin/docker-machine -L https://github.com/docker/machine/releases/download/v0.12.2/docker-machine-Linux-x86_64
RUN chmod +x /usr/local/bin/docker-machine

ADD https://storage.googleapis.com/docker-machine-drivers/docker-machine-driver-otc /usr/local/bin/docker-machine-driver-otc
RUN chmod +x /usr/local/bin/docker-machine-driver-otc

ADD _output/node-controller /node-controller
