# Inference system optimizer

The inference system optimizer assigns GPU types to inference model servers and decides on the number of replicas for each model for a given request traffic load and classes of service, as well as the batch size. ([slides](docs/slides/inferno-dynamic.pdf))

The light optimizer provided in this repository uses a simple greedy algorithm, as opposed to using a solver as in [optimizer](https://github.com/llm-inferno/optimizer).

## Building

```bash
docker build -t quay.io/atantawi/inferno-optimizer-light:latest . --load
```

## Running

### I. Optimizer only

There are two ways to run the optimizer.

1. **Direct function calls**: An example is provided in [main.go](demos/main/main.go).

    First, populate sample data.

    ```bash
    git submodule init
    git submodule update
    ```

    Then, run the demo.

    ```bash
    cd demos/main
    go run main.go
    ```

2. **REST API server**: The optimizer may run as a REST API server ([steps](#steps-to-run-the-optimizer-as-a-rest-api-server)).

### II. Optimized auto-scaler

One may run the optimizer as part of an auto-scaling control system, in one of two ways.

1. **Kubernetes controller**: Running in a Kubernetes cluster and using custom resources and a Kubernetes runtime controller, the optimizer may be excercised in reconciliation to updates to the Optimizer custom resource ([reference](https://github.com/llm-inferno/controller)).

2. **Optimization control loop**: The control loop comprises (1) a Collector to get data about the inference servers through Prometheus and server deployments, (2) an Optimizer to make decisions, (3) an Actuator to realize such decisions by updating server deployments, and (4) a periodic Controller that has access to static and dynamic data. The [control loop](https://github.com/llm-inferno/control-loop) may run either externally or in a Kubernetes cluster.

### Steps to run the optimizer as a REST API server

The REST API specifications are [documented](rest-server/README.md).

Clone this repository and set environment variable `INFERNO_REPO` to the path to it.

#### Option A: Run externally

```bash
cd $INFERNO_REPO/cmd/optimizer
go run main.go [-F]
```

The default is to run the server in **Stateless** mode. Use the optional `-F` argument to run in **Statefull** mode. ([Description of modes](rest-server/README.md#rest-server-modes))

You may then curl [API commands](rest-server/README.md#commands-list) to `http://localhost:8080`.

#### Option B: Run in cluster

- Deploy optimizer as a deployment, along with a service on port `80`, in name space `inferno` in the cluster. (The deployment yaml file starts the server in a container with the `-F` flag.)

    ```bash
    cd $INFERNO_REPO/manifests/yamls
    kubectl apply -f deploy-optimizer.yaml
    ```

- Forward port to local host.

    ```bash
    kubectl port-forward service/inferno-optimizer -n inferno 8080:80
    ```

    You may then curl API commands (above) to `http://localhost:8080`.

- (Optional) Inspect logs.

    ```bash
    POD=$(kubectl get pod -l app=inferno-optimizer -n inferno -o jsonpath="{.items[0].metadata.name}")
    kubectl logs -f $POD -n inferno 
    ```

- Cleanup.

    ```bash
    kubectl delete -f deploy-optimizer.yaml
    ```

## Detailed description of the optimizer

![problem-scope](docs/figs/Slide5.png)

![timing-definitions](docs/figs/Slide30.png)

![request-batching](docs/figs/Slide6.png)

![token-time-fitting](docs/figs/Slide7.png)

![modeling-batching](docs/figs/Slide9.png)

![qn-model](docs/figs/Slide8.png)

![system-occupancy](docs/figs/Slide32.png)

![impact-batch](docs/figs/Slide33.png)

![target-service](docs/figs/Slide34.png)

Decision variables

For each pair of (class of service, model):

- gpuProfile: the GPU type allocated
- numReplicas: the number of replicas
- batchSize: the batch size, given continuous batching

## Specifications: Accelerators and models

![accelerators](docs/figs/Slide13.png)

![models](docs/figs/Slide14.png)

## Example 1: Unlimited accelerators

![unlimited-assign](docs/figs/Slide16.png)

![unlimited-perf](docs/figs/Slide17.png)

## Example 2: Load change - Unlimited accelerators

![unlimited-change-assign](docs/figs/Slide19.png)

![unlimited-change](docs/figs/Slide20.png)

![unlimited-change-perf](docs/figs/Slide21.png)

## Example 3: Limited accelerators

![limited-count](docs/figs/Slide22.png)

![limited-assign](docs/figs/Slide23.png)

![limited-perf](docs/figs/Slide24.png)

## Example 4: Load change - Limited accelerators

![limited-change-assign](docs/figs/Slide26.png)

![limited-change](docs/figs/Slide27.png)

![limited-change-perf](docs/figs/Slide28.png)
