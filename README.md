# Cloud Est 2021 - Demo Controller
## `Coder un controller Kubernetes c'est bien, le tester c'est mieux`

Conference deck is available [here](https://speakerdeck.com/artifakt-io/cloud-est-2021)

This repository contains all code used during the talk.

Feel free to clone it, play with the code and get inspiration if you want to write a controller

### Quick install

#### Prerequisite

You must have a working Kubernetes cluster, `minikube` or `kind` are enough

:warning: If you are using a GKE cluster you must add a library to allow the controller to connect to the cluster

`cmd/main.go`

``` go
import (
    ...
    _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)
```

If you updated `pkg/apis` don't forget to run
```
make gen
```

Locally the controller will share the same kubernetes profile than yours in your current context, this profile must have enough RBAC

### Install CRD on your cluster

```
make install
```

### Start the controller

```
make run
```

:warning: Go code is compiled with a `darwin` architecture, if you are on linux, update `GOOS` flag
```
GOOPTS=GOARCH=amd64 CGO_ENABLED=0 GOOS=linux
```

You can deploy an example application provided into `examples`
```
kubectl apply -f examples/app.yaml
```

### Run the test suite

```
make test
```
