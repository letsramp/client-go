/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/letsramp/client-go/types"
	"github.com/letsramp/client-go/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const RequirementPath = "requirements.txt"

type Request struct {
	*types.TestRequest
	EndpointDescription *types.EndpointDescription
}

type Scenario struct {
	TestScenario        *types.TestScenario
	Requests            []*Request
	EndpointDescription *types.EndpointDescription
}

func NewScenario(name string) *Scenario {
	return &Scenario{
		TestScenario: &types.TestScenario{
			Name: name,
		},
	}
}

func (s *Scenario) SetRequest(request *Request) *types.TestStep {
	step := &types.TestStep{
		RequestName: request.Name,
	}
	s.TestScenario.Steps = append(s.TestScenario.Steps, step)
	s.Requests = append(s.Requests, request)
	return step
}

func (s *Scenario) SetAssertEqual(str1, str2 string) {
	s.TestScenario.Steps = append(s.TestScenario.Steps, &types.TestStep{
		Assert: fmt.Sprintf("%s == \"%s\"", str1, str2),
	})
}

func NewRequest(name string, endpointDesc *types.EndpointDescription, methodName, blob string) *Request {
	return &Request{
		TestRequest: &types.TestRequest{
			Name:       name,
			MethodName: methodName,
			Value: &types.Value{
				Blob: blob,
			},
		},
		EndpointDescription: updateEndpoint(endpointDesc, methodName),
	}
}

func (r *Request) SetGraphqlQuery(query *types.GraphqlParam) error {
	r.TestRequest.GraphqlParam = query
	return nil
}

func (r *Request) SetPythonFunction(pythonFunction, pythonPath string) error {
	if r.TestRequest.Value == nil {
		r.TestRequest.Value = &types.Value{}
	}
	if pythonFunction != "" {
		r.TestRequest.Value.Python = pythonFunction
	}
	if pythonPath != "" {
		data, err := utils.ReadFile("", pythonPath)
		if err != nil {
			return fmt.Errorf("failed to read python file: %w", err)
		}
		r.TestRequest.Value.Python = string(data)
	}
	return nil
}

func (r *Request) SetJSFunction(jsFunction, jsPath string) error {
	if r.TestRequest.Value == nil {
		r.TestRequest.Value = &types.Value{}
	}
	if jsFunction != "" {
		r.TestRequest.Value.Javascript = jsFunction
	}
	if jsPath != "" {
		data, err := utils.ReadFile("", jsPath)
		if err != nil {
			return fmt.Errorf("failed to read JS file: %w", err)
		}
		r.TestRequest.Value.Javascript = string(data)
	}
	return nil
}

func (r *Request) SetHeader(header map[string]string) {
	r.TestRequest.Value.Headers = header
}

func (r *Request) SetParams(params []*types.RestParam) {
	r.TestRequest.Value.Params = params
}

func (r *Request) SetCookies(cookies map[string]string) {
	r.TestRequest.Value.Cookies = cookies
}

func (r *Request) SetVars(vars map[string]interface{}) {
	r.TestRequest.Vars = vars
}

func generateTestPostRequest(scenario []*Scenario, address string, testName string) (*types.TestPostRequest, error) {
	testDesc := &types.TestDescription{
		Version: "v1",
		Test: &types.Test{
			Name:     testName,
			Patterns: []*types.TestPattern{},
		},
		Requests:  []*types.TestRequest{},
		Scenarios: []*types.TestScenario{},
	}
	//create mock object
	testerMockDesc := &types.MockDescription{
		Version:   "v1",
		Services:  []*types.Service{},
		Endpoints: []*types.Endpoint{},
	}

	for _, s := range scenario {
		testDesc.Test.Patterns = append(testDesc.Test.Patterns, &types.TestPattern{
			ScenarioName: s.TestScenario.Name,
			Start:        0,
		})
		testDesc.Scenarios = append(testDesc.Scenarios, s.TestScenario)

		for _, r := range s.Requests {
			testerMockDesc.Endpoints = addUniqueEndpoints(r.EndpointDescription, testerMockDesc.Endpoints)
			testerMockDesc.Services = addUniqueServices(r.EndpointDescription, testerMockDesc.Services)
			r.TestRequest.EndpointName = r.EndpointDescription.Endpoints[0].Name
			testDesc.Requests = append(testDesc.Requests, r.TestRequest)
		}
	}

	log.Infof("collecting protocol files for tests")
	fileInfo, err := generateTesterFileInfo(testerMockDesc)
	if err != nil {
		return nil, err
	}

	if rPath := testDesc.Test.RequirementPath; rPath != "" {
		data, err := os.ReadFile(rPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read requirement file: %w", err)
		}

		fileInfo[RequirementPath] = data
	}

	return &types.TestPostRequest{
		Description: testDesc,
		MockDesc:    testerMockDesc,
		Files:       fileInfo,
	}, nil
}

func generateTesterFileInfo(tester *types.MockDescription) (map[string][]byte, error) {
	// path to content map
	files := make(map[string][]byte)

	var loadFile = func(protocol types.ProtocolType, path string) error {
		if (protocol == types.Grpc || protocol == types.Thrift) && path != "" {
			if _, ok := files[path]; ok {
				log.Infof("same file is already loded")
				return nil
			}

			// load contents
			content, err := utils.ReadFile(viper.GetString("path"), path)
			if err != nil {
				log.Errorf("failed to read file %s: %v", path, err)
				return err
			}
			if err != nil {
				log.Errorf("failed to read file %s: %v", path, err)
				return err
			}
			files[path] = content
		}
		return nil
	}

	serviceProtocolMap := make(map[string]types.ProtocolType)

	for _, service := range tester.Services {
		serviceProtocolMap[service.Name] = service.Protocol
	}

	for _, endpoint := range tester.Endpoints {
		if endpoint.Defined != nil {
			path := endpoint.Defined.Path
			protocol := serviceProtocolMap[endpoint.ServiceName]
			if err := loadFile(protocol, path); err != nil {
				return nil, err
			}
		}
	}

	return files, nil
}

func parseCurlOutput(output string) (*types.TestStatusResponse, int, error) {
	lines := strings.Split(output, "\n")

	status, err := strconv.Atoi(strings.Fields(lines[0])[1])
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse http status %s: %w", lines[0], err)
	}

	if len(lines) <= 5 {
		return nil, status, nil
	}

	resp := strings.Join(lines[5:], "\n")
	response := &types.TestStatusResponse{}

	if err := json.Unmarshal([]byte(resp), &response); err != nil {
		log.Errorf("failed to parse response from tester \"%s\": %v", resp, err)
		return nil, status, fmt.Errorf("failed to parse response from tester %s: %w", resp, err)
	}

	return response, status, nil
}
