# client-go

`client-go` is a Golang package that provides a client for interacting with the [Skyramp](https://skyramp.dev/) module. It empowers you to deploy Skyramp workers, apply mock configurations, and initiate test scenarios in various environments, including Docker and Kubernetes.

## Installation

To install the `skyramp/client-go` package, use the following command:

```bash
go get -u github.com/letsramp/client-go
```

## Usage

Upon successful installation of Skyramp, introduce it to your project by adding the following import statements in your Golang code:

```go
import (
	"github.com/letsramp/client-go/client"
	"github.com/letsramp/client-go/lcm"
	"github.com/letsramp/client-go/types"
)
```

### Configuration

The `client` package is the main entry point for interacting with the Skyramp module. It facilitates applying configurations, starting test scenarios, and deploying/deleting workers. You can configure the `client` for either Kubernetes (local cluster or existing cluster) or deploying the Skyramp worker.

#### Kubernetes Configuration

First, set up the `client` with a Kubernetes cluster.

**Example: Provision Local Cluster with Skyramp**
```go
// Create the Kind cluster
err := createKINDCluster(clusterName, types.KindNodeImage, kubeconfig)

// Create a KubernetesClient
k8sClient = client.NewKubernetesClient(
    kubeconfig,
    k8sContext,
    clusterName,
    namespace,
)
```

Once configured with a Kubernetes cluster, deploy the Skyramp Worker in-cluster for applying mocks and running tests.

**Example: Deploy Skyramp Worker**
```go
err = k8sClient.InstallWorker()
```

#### Docker Configuration

For the Docker setting, configure a `DockerClient` and deploy the Skyramp Worker in the network of your choice for applying mocks and running tests.

**Example: Run Skyramp Worker in Docker Network**
```go
// Create a DockerClient
dockerClient = client.NewDockerClient("localhost:35142", "echo_default", 35142)

// Run Skyramp Worker in Docker network
err = dockerClient.InstallWorker()
```

### Mocking

To apply mock responses to the `DockerClient` or `KubernetesClient`, create `Service` and `Endpoint` objects (currently supported: `GrpcEndpoint` and `RestEndpoint`). Configure a `ResponseValue` for the method you wish to mock, passing through a dynamic `javascriptFunction` (a handler function) or a JSON `blob`.

Here is an example flow of creating a REST Mock Configuration from a `RestEndpoint` object:

**Example: Create REST Mock Configuration**
```go
// Define service configuration in a Service object
service := &types.Service{
    Name:         "echo",
    Protocol:     types.Rest,
    ServiceAlias: "echo",
    Port:         12346,
}

// Create a RestEndpoint
restEndpoint := client.NewRestEndpoint("echo-get", "/echo/{ msg }", "", "", service)

// Create a ResponseValue with a blob mock response
response := client.NewResponseValue(
    "echo_get",
    restEndpoint,
    "GET",
    `{
        "message": "ping pong"
    }`,
    // Define traffic configuration in the ResponseValue object
    &types.TrafficConfig{
        LossPercentage: 0,
        DelayConfig: &types.TrafficDelayConfig{
            MinDelay: 100,
            MaxDelay: 200,
        },
    },
)

// Apply the mock response to the Kubernetes Client
err = k8sClient.MockerApply([]*client.ResponseValue{response}, nil)
```

Here is an example flow of creating a gRPC Mock Configuration from a `GrpcEndpoint` object:

**Example: Create gRPC Mock Configuration**
```go 
// Define service configuration in a Service object
service := &types.Service{
    Name:         "helloworld",
    Protocol:     types.Grpc,
    ServiceAlias: "helloworld",
    Port:         50051,
}

// Create a GrpcEndpoint
grpcEndpoint := client.NewGrpcEndpoint("helloworld-endpoint", "Greeter", "../testdata/pb/helloworld.proto", "", service)

// Create a ResponseValue for the mock
response := client.NewResponseValue(
    "mock1",
    grpcEndpoint,
    "SayHello",
    "",
    nil,
)

// Define handler JavaScript function as the dynamic mock response
response.SetJSFunction(`function handler(req) {
    return {
        value: {
            message: req.value.name + "temp"
        }
    }
    }`, "")

// Apply the mock response to the KubernetesClient and specify the traffic configuration
err = k8sClient.MockerApply(
    []*client.ResponseValue{response},
    &types.TrafficConfig{
        LossPercentage: 0,
        DelayConfig: &types.TrafficDelayConfig{
            MinDelay: 100,
            MaxDelay: 200,
        },
    },
)
```

### Testing

To configure test requests to apply to the `KubernetesClient` or `DockerClient`, create `Service` and `Endpoint` objects (currently supported: `GrpcEndpoint` and `RestEndpoint`). Configure a `Scenario` built from `AssertEqual`s and `RequestValue`s for the method you wish to test, passing through a dynamic `javascriptFunction` (a handler function) or a JSON `blob`.

Here is an example flow of testing REST requests with a `RestEndpoint` object:

**Example: Test Assert Scenario (REST)**
```go
// Define service configuration in a Service object
service := &types.Service{
    Name:         "echo",
    Protocol:     types.Rest,
    ServiceAlias: "echo",
    Port:         12346,
}

// Create a RestEndpoint
restEndpoint := client.NewRestEndpoint("echo-get", "/echo/{ msg }", "", "", service)

// Create a REST request
equest := client.NewRequest(
    "echo_get",
    restEndpoint,
    "GET",
    "")
// Add request headers
headers := make(map[string]string)
headers["key"] = "value"
request.SetHeader(headers)
// Add REST parameters
params := make([]*types.RestParam, 0)
params = append(params, &types.RestParam{
    Name:  "msg",
    In:    "path",
    Value: "ping",
})
request.SetParams(params)

// Create a Scenario for testing
scenario := client.NewScenario("echo_scenario")

// Add the REST request to the scenario
step := scenario.SetRequest(request)

// Start the test scenario
err = k8sClient.TesterStart([]*client.Scenario{scenario}, "k8s_echo_test", nil, nil)
```

**Example: Test Assert Scenario (gRPC)**
```go
// Create a gRPC request
request := client.NewRequest(
    "request1",
    grpcEndpoint,
    "SayHello",
    "")

// Set a handler for dynamic gRPC test request
request.SetJSFunction(`i = 0
    function handler() {
        i++
        return {
            value: {
                name: vars.name + i
            }
        }
    }`,
    "")
// Set variables for dynamic gRPC test request
request.SetVars(map[string]interface{}{
    "name": "name",
})

// Create a Scenario for gRPC testing
scenario := client.NewScenario("dynamic_scneario")

// Add the gRPC request to the scenario
step1 := scenario.SetRequest(request)

// Add an assertion to the scenario
scenario.SetAssertEqual(step1.GetResponseValue("message"), "name1temp")

err = k8sClient.TesterStart([]*client.Scenario{scenario}, "k8s_dynamic_test", nil, nil)
```

## License

This Golang package is licensed under the MIT license.
