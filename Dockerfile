FROM debian:jessie
LABEL maintainer "henrik@loodse.com"

RUN apt-get update && apt-get install -y ca-certificates

RUN curl -o /usr/local/bin/docker-machine -L https://github.com/docker/machine/releases/download/v0.12.2/docker-machine-Linux-x86_64
RUN chmod +x /usr/local/bin/docker-machine

ADD ./controller /
