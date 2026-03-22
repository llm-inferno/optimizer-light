# A REST API Server for the Optimizer

The host name and port for the server are specified as environment variables `INFERNO_HOST` and `INFERNO_PORT`, respectively. If not set, the default server is at `localhost:8080`.

## Data Format

The following data is needed by the Optimizer (Declarations described [types](../pkg/config/types.go)).

1. **Accelerator data**: For all accelerators, the specification, such as name, type, cost, and other attributes of an accelerator. An example follows.

    ```json
    { 
        "accelerators": [
            {
                "name": "A100",
                "type": "A100",
                "multiplicity": 1,
                "power" : {
                    "idle": 150,
                    "full": 400,
                    "midPower": 320,
                    "midUtil": 0.6
                },
                "cost": 40.00
            },
            {
                "name": "G2",
                "type": "G2",
                "multiplicity": 1,
                "power" : {
                    "idle": 180,
                    "full": 600,
                    "midPower": 500,
                    "midUtil": 0.6
                },
                "cost": 25.00
            },
            {
                "name": "4xA100",
                "type": "A100",
                "multiplicity": 4,
                "power" : {
                    "idle": 600,
                    "full": 1600,
                    "midPower": 1280,
                    "midUtil": 0.6
                },
                "cost": 160.00
            }
        ]
    }
    ```

1. **Capacity data**: For all accelerator types, a count of available units of that type. An example follows.

    ```json
    { 
        "count": [
            {
                "type": "G2",
                "count": 256
            },
            {
                "type": "A100",
                "count": 128
            }
        ]
    }
    ```

1. **Model data**: For all models, a collection of performance data for pairs of model and accelerators. An example follows.

    ```json
    {
        "models": [
            {
            "name": "granite_13b",
            "acc": "A100",
            "accCount": 1,
            "maxBatchSize": 32,
            "atTokens": 512,
            "perfParms": {
                "alpha": 20.58,
                "beta": 0.41,
                "gamma": 0.0021
            }
            },
            {
            "name": "granite_13b",
            "acc": "G2",
            "accCount": 1,
            "maxBatchSize": 38,
            "atTokens": 512,
            "perfParms": {
                "alpha": 17.15,
                "beta": 0.34,
                "gamma": 0.0017
            }
            },
            {
            "name": "llama_70b",
            "acc": "G2",
            "accCount": 2,
            "maxBatchSize": 6,
            "atTokens": 512,
            "perfParms": {
                "alpha": 22.84,
                "beta": 5.89,
                "gamma": 0.0295
            }
            }
        ]
    }
    ```

    Performance data includes

   - `accCount`: number of accelerator (cards)
   - `maxBatchSize`: maximum batch size to use, beyond which performance deteriorates
   - `atTokens`: average number of tokens used when determining the `maxBatchSize`
   - `perfParms`: performance parameters `alpha`, `beta`, and `gamma` (in msec) of the linear approximation of iteration time as a function of computed tokens and transferred tokens per batch (n), *iterationTime = alpha + beta . computedTokens + gamma . transferredTokens*

1. **Service class data**: For all service classes, the specification, such as name, priority, and SLO targets for a service class. An example follows.

    ```json
    {
        "serviceClasses": [
            {
                "name": "Premium",
                "priority": 1,
                "modelTargets": [
                    {
                        "model": "granite_13b",
                        "slo-itl": 40,
                        "slo-ttft": 1000
                    },
                    {
                        "model": "llama_70b",
                        "slo-itl": 80,
                        "slo-ttft": 1000
                    }
                ]
            },
            {
                "name": "Bronze",
                "priority": 2,
                "modelTargets": [
                    {
                        "model": "granite_13b",
                        "slo-itl": 80,
                        "slo-ttft": 2000
                    }
                ]
            },
            {
                "name": "Batch2K",
                "priority": 4,
                "modelTargets": [
                    {
                        "model": "mixtral_8_7b",
                        "slo-tps": 4000
                    }
                ]
            }
        ]
    }
    ```

    The service class specification includes

    - `priority`: an integer between 1 (highest priority) and 100 (lowest priority) - if unspecified, lowest priority is assumed
    - `modelTargets`: target SLOs for models

      - `name`: name of model
      - `slo-itl`: target SLO for ITL (msec)
      - `slo-ttft` target SLO TTFT, including queueing time (msec)
      - `slo-tps` target SLO for throughput (tokens/sec)

1. **Server data**: For all inference servers, the name of the server, the model and service class it serves (currently, assuming a single model and service class per server), an option to not change the accelerator, a minimum number of replicas, a maximum batch size, and current and desired allocations. The current allocation reflects the state of the server and the desired allocation is provided by the Optimizer (as a solution to an optimization problem). An allocation includes accelerator, number of replicas, maximum batch size, cost, and observed or anticipated average ITL and TTFT times, as well as load data. The load data includes statistical metrics about request arrivals and message lengths (number of input and output tokens). An example follows.

    ```json
    {
        "servers": [
            {
                "name": "Premium-granite_13b",
                "class": "Premium",
                "model": "granite_13b",
                "keepAccelerator": false,
                "minNumReplicas": 1,
                "currentAlloc": {
                    "accelerator": "A100",
                    "numReplicas": 1,
                    "maxBatch": 16,
                    "cost": 40,
                    "itlAverage": 25.2,
                    "ttftAverage": 726.5,
                    "load": {
                        "arrivalRate": 100,
                        "avgInTokens": 128,
                        "avgOutTokens": 999
                    }
                },
                "desiredAlloc": {
                    "accelerator": "G2",
                    "numReplicas": 2,
                    "maxBatch": 19,
                    "cost": 46,
                    "itlAverage": 21.16437,
                    "ttftAverage": 102.09766,
                    "load": {
                        "arrivalRate": 60,
                        "avgInTokens": 96,
                        "avgOutTokens": 1024
                    }
                }
            }
        ]
    }
    ```

1. **Optimizer data**: Optional flags for the Optimizer. An example follows.

    ```json
    {
        "optimizer": {
            "unlimited": false,
            "delayedBestEffort": false,
            "saturationPolicy" : "None"
        }
    }
    ```

    The flags are as follows.

    - `unlimited`: The available number of accelerator types is unlimited (used in capacity planning mode), as opposed to being limited to the specified number (used in cluster mode).
    - `delayedBestEffort`: Delay best effort allocation after attempting allocation to all priority groups.
    - `saturationPolicy`: Set an allocation policy under saturated condition.

      - ***None***: no additional allocation beyond satisfying SLOs
      - ***PriorityExhaustive***: allocating exhaustively to servers in priority ordering
      - ***PriorityRoundRobin***: allocating in round-robin fashion within priority groups
      - ***RoundRobin***: allocating in round-robin fashion across all servers

The output of the Optimizer is an Allocation Solution, in addition to updating the desired allocation of all servers.

**Allocation solution data**: A map from server name to Allocation Data. An example follows.

```json
{
    "allocations": {
        "Premium-granite_13b": {
            "accelerator": "G2",
            "numReplicas": 2,
            "maxBatch": 19,
            "cost": 46,
            "itlAverage": 21.16437,
            "ttftAverage": 102.09766,
            "load": {
                "arrivalRate": 60,
                "avgInTokens": 96,
                "avgOutTokens": 1024
            }
        }
    }
}
```

## Commands List

| Verb | Command | Parameters | Returns | Description |
| --- | :---: | :---: | :---: | --- |
| **Accelerator specs** | | | | |
| /setAccelerators | POST | AcceleratorData |  | set specs for all accelerators |
| /getAccelerators | GET |  | AcceleratorData | get specs for all accelerators |
| /getAccelerator | GET | name | AcceleratorSpec | get specs for named accelerator |
| /addAccelerator | POST | AcceleratorSpec |  | add spec for an accelerator |
| /removeAccelerator | GET | name |  | remove the named accelerator |
| **Accelerator type counts** | | | | |
| /setCapacities | POST | CapacityData |  | set counts for all accelerator types |
| /getCapacities | GET |  | CapacityData | get counts for all accelerator types |
| /getCapacity | GET | name | AcceleratorCount | get count for an accelerator type |
| /setCapacity | POST | AcceleratorCount |  | set a count to an accelerator type |
| /removeCapacity | GET | name |  | remove count of an accelerator type |
| **Model data** | | | | |
| /setModels | POST | ModelData |  | set data for models |
| /getModels | GET |  | model names | get names of all models |
| /getModel | GET | name | ModelData | get data for a model |
| /addModel | GET | name |  | add a model by name |
| /removeModel | GET | name |  | remove the data of a model |
| **Service class data** | | | | |
| /setServiceClasses | POST | ServiceClassData |  | set data for service classes |
| /getServiceClasses | GET |  | ServiceClassData | get data for all service classes |
| /getServiceClass | GET | name | ServiceClassSpec | get data for a service class |
| /addServiceClass | GET | name/priority | ServiceClassSpec | add a service class by name |
| /removeServiceClass | GET | name | ServiceClassSpec | remove the data of a service class |
| **Service class targets** | | | | |
| /addServiceClassModelTargets | POST | ServiceClassSpec | ServiceClassSpec | add model targets to a service class |
| /getServiceClassModelTarget | GET |  service class name / model name | ModelTarget | get the SLO targets for a service class and model pair |
| /removeServiceClassModelTarget | GET |  service class name / model name | ModelTarget | remove the SLO targets for a service class and model pair |
| **Server data** | | | | |
| /setServers | POST | ServerData |  | set data for servers |
| /getServers | GET |  | ServerData | get data for all servers |
| /getServer | GET | name | ServerSpec | get spec for a server |
| /addServer | POST | ServerSpec |  | add a server spec |
| /removeServer | GET | name |  | remove the data of a server |
| **Model Accelerator perf data** | | | | |
| /getModelAcceleratorPerf | GET |  model name / accelerator name | ModelAcceleratorPerfData | get the perf data for a model and accelerator pair |
| /addModelAcceleratorPerf | POST | ModelAcceleratorPerfData |  | add perf data for a model and accelerator pair |
| /removeModelAcceleratorPerf | GET |  model name / accelerator name | | remove the perf data for a model and accelerator pair |
| **Optimization** | | | | |
| /optimize | POST | OptimizerData | AllocationSolution | optimize given all system data provided and return optimal solution |
| /optimizeOne | POST | SystemData | AllocationSolution | optimize for system data and return optimal solution (stateless, all system data provided with command) |

## REST Server modes

There are two modes to run the server.

1. **Statefull**: All commands listed above are supported. The server keeps the state as data about various entities, allowing additions, updates, and deletions. Optimization is performed on the system as given by the state at the time `/optimize` is called.
2. **Stateless**: Optimization is performed using the provided system data when `/optimizeOne` is called. Optionally, any command prefixed with `/get` may be called afterwards to get data about various entities.
