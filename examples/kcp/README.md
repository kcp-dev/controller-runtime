# controller-runtime-example
An example project that is multi-cluster aware and works with [kcp](https://github.com/kcp-dev/kcp)

## Description

In this example, we intentionally not using advanced kubebuilder patterns to keep the example simple and easy to understand.
In the future, we will add more advanced examples. Example covers 3 parts:
1. KCP bootstrapping - creating APIExport & consumer workspaces
   1. Creating WorkspaceType for particular exports
2. Running controller with APIExport aware configuration and reconciling multiple consumer workspaces


This example contains an example project that works with APIExports and multiple kcp workspaces. It demonstrates
two reconcilers:

1. ConfigMap
   1. Get a ConfigMap for the key from the queue, from the correct logical cluster
   2. If the ConfigMap has labels["name"], set labels["response"] = "hello-$name" and save the changes
   3. List all ConfigMaps in the logical cluster and log each one's namespace and name
   4. If the ConfigMap from step 1 has data["namespace"] set, create a namespace whose name is the data value.
   5. If the ConfigMap from step 1 has data["secretData"] set, create a secret in the same namespace as the ConfigMap,
      with an owner reference to the ConfigMap, and data["dataFromCM"] set to the data value.

2. Widget
   1. Show how to list all Widget instances across all logical clusters
   2. Get a Widget for the key from the queue, from the correct logical cluster
   3. List all Widgets in the same logical cluster
   4. Count the number of Widgets (list length)
   5. Make sure `.status.total` matches the current count (via a `patch`)

## Getting Started

### Running on kcp

1. Run KCP with the following command:

```sh
make kcp-server
```

From this point onwards you can inspect kcp configuration using kubeconfig:

```sh
export KUBECONFIG=.test/kcp.kubeconfig
```

1. Bootstrap the KCP server with the following command:

```sh
export KUBECONFIG=./.test/kcp.kubeconfig
make kcp-bootstrap
```

1. Run controller:

```sh
export KUBECONFIG=./.test/kcp.kubeconfig
make run-local
```

1. In separate shell you can run tests to exercise the controller:

```sh
export KUBECONFIG=./.test/kcp.kubeconfig
make test
```

### Uninstall resources
To delete the resources from kcp:

```sh
make test-clean
```



### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources until the desired state is reached.




