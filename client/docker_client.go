/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
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
	// Address is the Skyramp worker's daemon address.
	Address string

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

	return &DockerClient{
		Address:         workerAddress,
		NetworkName:     targetNetworkName,
		HostPort:        workerPort,
		WorkerImageRepo: types.SkyrampWorkerImage,
		WorkerImageTag:  types.SkyrampWorkerImageTag,
		TestConfig:      testConfig,
	}, nil
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

// TesterStart starts the test in the docker environment.
// It takes the test scenario, test name, global variables, and global headers as parameters.
// It returns an error if the test fails to start.
func (c *DockerClient) TesterStart(
	scenario []*Scenario,
	testName string,
	globalVars map[string]interface{},
	globalHeaders map[string]string,
) (*types.TestStatusResponse, error) {
	testRequest, err := generateTestPostRequest(scenario, c.Address, testName)
	testRequest.Description.Test.GlobalHeaders = globalHeaders
	testRequest.Description.Test.GlobalVars = globalVars
	if err != nil {
		return nil, fmt.Errorf("failed to generate test request: %w", err)
	}

	endpoint := getTestEndpointForStandalone(c.Address)

	requestByte, err := json.Marshal(testRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal test request: %w", err)
	}

	log.Infof("request %+v", testRequest)
	log.Infof("requestByte %s", string(requestByte))

	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(requestByte))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request for test: %w", err)
	}

	request.Header.Add("Content-Type", "application/json")

	client := http.Client{}

	resp, err := client.Do(request)
	if err != nil {
		log.Errorf("failed to start tests: %v", err)
		if urlErr, ok := err.(*url.Error); ok {
			return nil, fmt.Errorf("URL-related error occurred while starting test: %w", urlErr)
		}
		return nil, fmt.Errorf("failed to start tester: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read response from worker: %w", err)
	}

	var response *types.TestStatusResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response in worker %w", err)
	}
	testId := response.TestID

	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("worker rejected tester start %v", response.Error)
	}

	responseStatus := c.TestStatus(testId)
	if responseStatus.Status == types.TesterFailed {
		return nil, fmt.Errorf("tester failed: %s", responseStatus.Error)
	}
	return responseStatus, nil
}

func (c *DockerClient) MockerApply(response []*ResponseValue, trafficConfig *types.TrafficConfig) error {
	mockDescription, formMap, err := generateMockPostData("", c.Address, response, trafficConfig)
	if err != nil {
		return fmt.Errorf("failed to generate mock post data: %w", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if len(formMap) != 0 {
		for formName, fileNameMap := range formMap {
			for fileName, fileContent := range fileNameMap {
				part, err := writer.CreateFormFile(formName, fileName)
				if err != nil {
					return err
				}
				_, err = io.WriteString(part, fileContent)
				if err != nil {
					return err
				}
			}
		}
	}

	writer.Close()

	mockEndpoint := getMockEndpointForStandalone(c.Address)
	request, err := http.NewRequest(http.MethodPut, mockEndpoint, body)
	if err != nil {
		return fmt.Errorf("failed to generate PUT HTTP request for mocks: %w", err)
	}

	request.Header.Add("Content-Type", writer.FormDataContentType())

	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Do(request)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			return fmt.Errorf("URL-related error occurred while sending request to worker: %w", urlErr)
		}
		return fmt.Errorf("failed to send request to worker: %w", err)
	} else {
		if resp.StatusCode != http.StatusAccepted {
			log.WithField("status", resp.StatusCode).Debug("worker responded with unexpected status code")

			bodyBytes, respErr := io.ReadAll(resp.Body)
			if respErr != nil {
				return fmt.Errorf("failed to parse error from Skyramp worker: %w", respErr)
			} else if len(bodyBytes) == 0 {
				return fmt.Errorf("failed to apply mock")
			}

			return fmt.Errorf("failed to apply mock on Skyramp worker: %s", string(bodyBytes))
		}
	}

	// if standalone container, restart with new ports open
	dockerLcm, err := lcm.NewDockerLCM()
	if err == nil {
		worker, err := dockerLcm.FindContainerByBoundAddress(c.Address)
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

	waitForWorkerToBeReady(c.Address, "Applying mock configuration")
	return nil
}

// TestStatus returns the status of the test in the docker environment.
// It returns the test status response.
func (c *DockerClient) TestStatus(testId string) *types.TestStatusResponse {
	var testStatus *types.TestStatusResponse
	deadline := time.Now().Add(types.WorkerWaitTime)
retry:
	for retries := 0; time.Now().Before(deadline) && retries < maxRetries; retries++ {
		var status int
		var err error
		testStatus, status, err = c.GetTesterStatus(testId)
		if err != nil {
			testStatus = &types.TestStatusResponse{
				Status: types.TesterFailed,
				Error:  fmt.Errorf("failed to parse response from worker: %w", err).Error(),
			}
			break
		}

		if status != http.StatusOK {
			testStatus = &types.TestStatusResponse{
				Status: types.TesterFailed,
				Error:  "No test status available",
			}
			break
		}

		switch testStatus.Status {
		case types.TesterFailed:
			break retry
		case types.TesterStopped:
			testStatus.Message = fmt.Sprintf("Test stopped: %s", testStatus.Message)
			break retry
		case types.TesterFinished:
			break retry
		default:
			time.Sleep(retryInterval)
		}
	}
	return testStatus
}

// GetTesterStatus returns the status of the test in the docker environment.
// It returns the test status response, status code, and error.
func (c *DockerClient) GetTesterStatus(testId string) (*types.TestStatusResponse, int, error) {
	endpoint := getTestStatusEndpointForStandalone(c.Address, testId)

	resp, err := http.Get(endpoint)
	if err != nil {
		log.Errorf("failed to retrieve test status: %v", err)
		return nil, 0, fmt.Errorf("failed to retrieve test status: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		log.Errorf("failed to read response from worker: %v", err)
		return nil, 0, fmt.Errorf("failed to read response from worker: %v", err)
	}

	var response types.TestStatusResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, 0, fmt.Errorf("failed to read response from worker: %v", err)
	}

	return &response, resp.StatusCode, nil
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
func getMockEndpointForStandalone(address string) string {
	return fmt.Sprintf("http://%s/%s", address, types.WorkerMockConfigPath)
}

func getReadyzEndpoint(address string) string {
	return fmt.Sprintf("http://%s/%s", address, types.WorkerReadyzPath)
}

func getTestStatusEndpointForStandalone(address, testerId string) string {
	return fmt.Sprintf("http://%s/%s/%s", address, types.WorkerTestPath, testerId)
}
func getTestEndpointForStandalone(address string) string {
	return fmt.Sprintf("http://%s/%s", address, types.WorkerTestPath)
}
