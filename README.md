# Kube-machine - Node Controller

Unit tests: [![CircleCI](https://circleci.com/gh/kube-node/kube-machine/tree/master.svg?style=svg)](https://circleci.com/gh/kube-node/kube-machine/tree/master)

## Overview

This is a reference implementation of a Node controller based on [Docker machine](https://github.com/docker/machine)

The whole implementation is more a proof-of-concept.

## How it works

The node controller watches node resources and will create them based upon a node class at the cloud provider

## Usage

1. Deploy kube-machine in your cluster or run it locally
2. Adjust and create node class. See examples/NodeClass_do.yaml
3. Adjust and create node objects examples/Node1_coreos.yaml
4. Wait and check the kube-machine logs.

### CLI
```bash
Usage of ./controller:
      --alsologtostderr                  log to standard error as well as files
      --kubeconfig string                Path to kubeconfig file with authorization and master location information.
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files
      --master string                    The address of the Kubernetes API server (overrides any value in kubeconfig)
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

## Building

```bash
git clone git@github.com:kube-node/kube-machine.git
dep ensure
```
