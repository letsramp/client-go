/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/letsramp/client-go/lcm"
	"github.com/letsramp/client-go/types"

	log "github.com/sirupsen/logrus"
)

const (
	maxRetries    = 5
	retryInterval = 5 * time.Second
)

// DockerClient represents a client configuration for interacting with Docker.
type DockerClient struct {
	ClientBase

	// NetworkName is the name of the Docker network where the Skyramp worker gets installed.
	NetworkName string

	// HostPort is the port to be used by the Skyramp worker.
	HostPort int

	// WorkerImageRepo is the repository for the worker image.
	WorkerImageRepo string

	// WorkerImageTag is the tag associated with the worker image.
	WorkerImageTag string

	// TestConfig is the test configuration for the client.
	TestConfig *TestConfig
}

// NewDockerClient creates a new Docker client.
// It takes the address, target network name, and host port as parameters.
func NewDockerClient(address, targetNetworkName string, testConfig *TestConfig) (*DockerClient, error) {
	var workerAddress string
	var workerPort int

	if testConfig == nil {
		return nil, fmt.Errorf("testConfig is required for DockerClient")
	}
	if strings.Contains(address, ":") {
		// address is in the format host:port
		var err error
		workerAddress = address
		var portStr string
		_, portStr, err = net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("Invalid worker address: [%s] %v", address, err)
		}

		workerPort, err = strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("Invalid port number %s: %v", portStr, err)
		}
	} else {
		// address is in the format host
		workerPort = types.WorkerManagementPort
		workerAddress = fmt.Sprintf("%s:%d", address, workerPort)
	}

	ret := &DockerClient{
		NetworkName:     targetNetworkName,
		HostPort:        workerPort,
		WorkerImageRepo: types.SkyrampWorkerImage,
		WorkerImageTag:  types.SkyrampWorkerImageTag,
		TestConfig:      testConfig,
	}

	ret.address = workerAddress
	return ret, nil
}

// InstallWorker installs the worker in the docker environment.
// It returns an error if the worker installation fails.
func (c *DockerClient) InstallWorker() error {
	return lcm.RunSkyrampWorkerInDockerNetwork(c.WorkerImageRepo, c.WorkerImageTag, c.HostPort, c.NetworkName)
}

// UninstallWorker uninstalls the worker in the docker environment.
// It returns an error if the worker uninstallation fails.
func (c *DockerClient) UninstallWorker() error {
	return lcm.RemoveSkyrampWorkerInDocker(types.WorkerContainerName)
}

func (c *DockerClient) Cleanup() {
	if c.TestConfig.DeployWorker {
		_ = c.UninstallWorker()
		time.Sleep(DefaultDeployTime)
	}
}

// SetWorkerImage overrides the default worker image.
// It takes the image and tag as parameters.
func (c *DockerClient) SetWorkerImage(image, tag string) {
	c.WorkerImageRepo = image
	c.WorkerImageTag = tag
}

func (c *DockerClient) MockerApply(response []*ResponseValue, trafficConfig *types.TrafficConfig) error {
	mockDescription, err := c.MockerApplyCommon(response, trafficConfig)
	if err != nil {
		return err
	}

	// if standalone container, restart with new ports open
	dockerLcm, err := lcm.NewDockerLCM()
	if err == nil {
		worker, err := dockerLcm.FindContainerByBoundAddress(c.address)
		// possibly, standalone process, not docker container
		if err != nil {
			log.Infof("apply succeeded, please restart the worker")
			return nil
		}

		// possibly, standalone container
		if lcm.IsInDefaultBridgeNetwork(worker) {
			// attempt to restart the container with new port bindings
			portsMap := make(map[int]bool)
			portsToOpen := make([]int, 0)
			for _, s := range mockDescription.Services {
				if portsMap[s.Port] {
					return fmt.Errorf("multiple services to mock with same port. please correct port conflicts")
				}
				portsToOpen = append(portsToOpen, s.Port)
				portsMap[s.Port] = true
			}
			log.Infof("new ports %v", portsToOpen)
			err = dockerLcm.RestartWithNewPorts(worker, portsToOpen)
			if err != nil {
				return fmt.Errorf("failed to restart the container with ports %v: %w", portsToOpen, err)
			}
		}
	} else {
		// possibly, standalone process, not docker container
		log.Infof("apply succeeded, please restart the worker")
		return nil
	}

	waitForWorkerToBeReady(c.address, "Applying mock configuration")
	return nil
}

func (c *DockerClient) GetTestConfig() *TestConfig {
	return c.TestConfig
}

// in a loop continuously wait until `readyz` endpoint is ready
func waitForWorkerToBeReady(address, msg string) {
	const workerWaitTime = 2 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), workerWaitTime)
	defer cancel()

	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for {
		exit := false
		select {
		case <-ctx.Done():
			log.Errorf("error occurred while waiting for container to be ready: %v", ctx.Err())
			exit = true
		case <-timer.C:
			resp, err := http.Get(getReadyzEndpoint(address))
			if err == nil && resp.StatusCode == http.StatusOK {
				exit = true
			}
		default:
			time.Sleep(1 * time.Second)
		}

		if exit {
			break
		}
	}
}

func getReadyzEndpoint(address string) string {
	return fmt.Sprintf("http://%s/%s", address, types.WorkerReadyzPath)
}
