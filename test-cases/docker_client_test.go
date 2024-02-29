/*
 * Copyright Skyramp Authors 2024
 */
package client_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/letsramp/client-go/client"
	"github.com/letsramp/client-go/types"
	"github.com/stretchr/testify/assert"
)

var (
	dockerClient         client.Client
	workerImg, workerTag string
)

func updateWorker(client client.Client) {
	workerImg = os.Getenv("WORKER_IMAGE_REPO")
	workerTag = os.Getenv("WORKER_IMAGE_TAG")
	if workerImg != "" && workerTag != "" {
		client.SetWorkerImage(workerImg, workerTag)
	}
}
func TestDockerEchoService(t *testing.T) {
	err := os.Chdir("../testdata")
	assert.Equal(t, nil, err)

	cmd := exec.Command("docker", "build", "-t", "echo:v0.0", "-f", "dockerfiles/echo/Dockerfile", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	assert.Equal(t, nil, err)

	cmd = exec.Command("docker", "compose", "-f", "compose/echo/docker-compose.yaml", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	assert.Equal(t, nil, err)

	dockerClient = client.NewDockerClient("localhost:35142", "echo_default", 35142)
	updateWorker(dockerClient)

	err = dockerClient.InstallWorker()
	assert.Equal(t, nil, err)

	service := &types.Service{
		Name:         "echo",
		Protocol:     types.Rest,
		ServiceAlias: "echo",
		Port:         12346,
	}

	restEndpoint := client.NewRestEndpoint("echo-get", "/echo/{ msg }", "", "", service)

	scenario := client.NewScenario("echo_test")

	request := client.NewRequest(
		"echo_get",
		restEndpoint,
		"GET",
		"")
	headers := make(map[string]string)
	headers["key"] = "value"

	request.SetHeader(headers)

	params := make([]*types.RestParam, 0)
	params = append(params, &types.RestParam{
		Name:  "msg",
		In:    "path",
		Value: "ping",
	})

	request.SetParams(params)

	step := scenario.SetRequest(request)
	assert.NotEqual(t, nil, step)
	scenario.SetAssertEqual(step.GetResponseValue("message"), "ping pong")

	status, err := dockerClient.TesterStart([]*client.Scenario{scenario}, "echo-test", nil, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, types.TesterStatusType("finished"), status.Status)

	dockerClient.UninstallWorker()
}

func TestDockerStandalone(t *testing.T) {
	dockerClient = client.NewDockerClient("localhost:35142", "", 35142)
	updateWorker(dockerClient)

	err := dockerClient.InstallWorker()
	assert.Equal(t, nil, err)

	service := &types.Service{
		Name:         "helloworld",
		Protocol:     types.Grpc,
		ServiceAlias: "helloworld",
		Port:         50052,
	}

	grpcEndpoint := client.NewGrpcEndpoint("helloworld-endpoint", "Greeter", "../testdata/pb/helloworld.proto", "localhost", service)

	response := client.NewResponseValue(
		"mock1",
		grpcEndpoint,
		"SayHello",
		"",
		nil,
	)

	response.SetPythonFunction(`
def handler(req):
    return SkyrampValue(
    value={"message": req.value.name + "temp"}
)
`, "")
	// mocker apply
	err = dockerClient.MockerApply(
		[]*client.ResponseValue{response},
		&types.TrafficConfig{
			LossPercentage: 0,
			DelayConfig: &types.TrafficDelayConfig{
				MinDelay: 100,
				MaxDelay: 200,
			},
		},
	)
	assert.Equal(t, nil, err)

	scenario := client.NewScenario("scenario1")

	request := client.NewRequest(
		"request1",
		grpcEndpoint,
		"SayHello",
		`{
            "name": "test"
        }`)
	step := scenario.SetRequest(request)
	assert.NotEqual(t, nil, step)
	scenario.SetAssertEqual(step.GetResponseValue("message"), "testtemp")

	status, err := dockerClient.TesterStart([]*client.Scenario{scenario}, "docker-standalone-test", nil, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, types.TesterStatusType("finished"), status.Status)

	dockerClient.UninstallWorker()
}

func TestDockerCompose(t *testing.T) {
	err := os.Chdir("../testdata")
	assert.Equal(t, nil, err)

	cmd := exec.Command("docker", "build", "-t", "helloworld:v0.0", "-f", "dockerfiles/helloworld/Dockerfile", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	assert.Equal(t, nil, err)

	cmd = exec.Command("docker", "compose", "-f", "compose/helloworld/docker-compose.yaml", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	assert.Equal(t, nil, err)

	dockerClient = client.NewDockerClient(
		"localhost:35142",
		"helloworld_default",
		35142,
	)
	updateWorker(dockerClient)

	err = dockerClient.InstallWorker()
	assert.Equal(t, nil, err)

	service := &types.Service{
		Name:         "helloworld",
		Protocol:     types.Grpc,
		ServiceAlias: "helloworld",
		Port:         50051,
	}

	grpcEndpoint := client.NewGrpcEndpoint("helloworld-endpoint", "Greeter", "../testdata/pb/helloworld.proto", "", service)

	scenario := client.NewScenario("scenario1")

	request := client.NewRequest(
		"request1",
		grpcEndpoint,
		"SayHello",
		`{ "name": "test"}`,
	)
	step := scenario.SetRequest(request)
	assert.NotEqual(t, nil, step)
	scenario.SetAssertEqual(step.GetResponseValue("message"), "Hello test")

	status, err := dockerClient.TesterStart([]*client.Scenario{scenario}, "docker-standalone-test", nil, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, types.TesterStatusType("finished"), status.Status)

	dockerClient.UninstallWorker()
}
