/*
 * Copyright Skyramp Authors 2025
 */
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/letsramp/client-go/types"

	log "github.com/sirupsen/logrus"
)

type ClientBase struct {
	backendName string
	address     string
}

func getMockEndpoint(address string) string {
	return fmt.Sprintf("http://%s/%s", address, types.WorkerMockConfigPath)
}

func getTestEndpoint(address, testerId string) string {
	if testerId != "" {
		return fmt.Sprintf("http://%s/%s/%s", address, types.WorkerTestPath, testerId)
	}
	return fmt.Sprintf("http://%s/%s", address, types.WorkerTestPath)
}

func (b *ClientBase) MockerApplyCommon(response []*ResponseValue, trafficConfig *types.TrafficConfig) (*types.MockDescription, error) {
	mockDescription, formMap, err := generateMockPostData("", b.address, response, trafficConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate mock post data: %w", err)
	}

	mockEndpoint := getMockEndpoint(b.address)
	log.Infof("Executing mocker apply on %s", mockEndpoint)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if len(formMap) != 0 {
		for formName, fileNameMap := range formMap {
			for fileName, fileContent := range fileNameMap {
				part, err := writer.CreateFormFile(formName, fileName)
				if err != nil {
					return nil, err
				}
				_, err = io.WriteString(part, fileContent)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	writer.Close()

	request, err := http.NewRequest(http.MethodPut, mockEndpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to generate put http request for mocks: %w", err)
	}

	request.Header.Add("Content-Type", writer.FormDataContentType())

	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Do(request)
	if err != nil {
		// TODO for docker apply, server could be disconnected from docker network,
		// making this http connection to close without receiving response.
		if _, ok := err.(*url.Error); !ok {
			return nil, fmt.Errorf("failed to send request to worker: %w", err)
		}
	} else {
		if resp.StatusCode != http.StatusAccepted {
			log.WithField("status", resp.StatusCode).Debug("worker responded with unexpected status code")

			bodyBytes, respErr := io.ReadAll(resp.Body)
			if respErr != nil {
				return nil, fmt.Errorf("failed to parse error from Skyramp worker: %w", respErr)
			} else if len(bodyBytes) == 0 {
				return nil, fmt.Errorf("failed to apply mock on Skyramp worker")
			}

			return nil, fmt.Errorf("failed to apply mock on Skyramp worker: %s", string(bodyBytes))
		}
	}

	return mockDescription, nil
}

func (b *ClientBase) TesterStart(
	scenario []*Scenario,
	testName string,
	globalVars map[string]interface{},
	globalHeaders map[string]string,
) (*types.TestStatusResponse, error) {
	testRequest, err := generateTestPostRequest(scenario, b.address, testName)
	testRequest.Description.Test.GlobalHeaders = globalHeaders
	testRequest.Description.Test.GlobalVars = globalVars
	if err != nil {
		return nil, fmt.Errorf("failed to generate test request: %w", err)
	}

	endpoint := getTestEndpoint(b.address, "")

	requestByte, err := json.Marshal(testRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal test request: %w", err)
	}

	log.Infof("request %+v", testRequest)
	log.Infof("requestByte %s", string(requestByte))

	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(requestByte))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request for test: %w", err)
	}

	request.Header.Add("Content-Type", "application/json")

	client := http.Client{}

	resp, err := client.Do(request)
	if err != nil {
		log.Errorf("failed to start tests: %v", err)
		return nil, fmt.Errorf("failed to start tester: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read response from worker: %w", err)
	}

	var response types.TestStatusResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response in worker %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("worker rejected tester start %v", response.Error)
	}

	responseStatus := b.TestStatus(response.TestID)
	if responseStatus.Status == types.TesterFailed {
		return nil, fmt.Errorf("tester failed: %s", responseStatus.Error)
	}
	return responseStatus, nil
}

func (b *ClientBase) GetTesterStatus(testerId string) (*types.TestStatusResponse, int, error) {
	endpoint := getTestEndpoint(b.address, testerId)

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

// TestStatus returns the status of the test in the docker environment.
// It returns the test status response.
func (b *ClientBase) TestStatus(testId string) *types.TestStatusResponse {
	var testStatus *types.TestStatusResponse
	deadline := time.Now().Add(types.WorkerWaitTime)
retry:
	for retries := 0; time.Now().Before(deadline) && retries < maxRetries; retries++ {
		var status int
		var err error
		testStatus, status, err = b.GetTesterStatus(testId)
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
