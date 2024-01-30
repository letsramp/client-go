/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/letsramp/client-go/types"
	"github.com/letsramp/client-go/utils"
)

const (
	mDescription = "mockDescription"
	mFiles       = "mockFiles"
	resources    = "resources.yaml"
)

type ResponseValue struct {
	*types.Response
	TrafficConfig       *types.TrafficConfig
	EndpointDescription *types.EndpointDescription
}

func NewResponseValue(name string, endpointDesc *types.EndpointDescription, methodName, blob string, trafficConfig *types.TrafficConfig) *ResponseValue {
	return &ResponseValue{
		Response: &types.Response{
			Name:       name,
			MethodName: methodName,
			Value: &types.Value{
				Blob: blob,
			},
		},
		EndpointDescription: updateEndpoint(endpointDesc, methodName),
		TrafficConfig:       trafficConfig,
	}
}

func (r *ResponseValue) SetPythonFunction(pythonFunction, pythonPath string) error {
	if r.Response.Value == nil {
		r.Response.Value = &types.Value{}
	}
	if pythonFunction != "" {
		r.Response.Value.Python = pythonFunction
	}
	if pythonPath != "" {
		data, err := utils.ReadFile("", pythonPath)
		if err != nil {
			return fmt.Errorf("failed to read python file: %w", err)
		}
		r.Response.Value.Python = string(data)
	}
	return nil
}

func (r *ResponseValue) SetJSFunction(jsFunction, jsPath string) error {
	if r.Response.Value == nil {
		r.Response.Value = &types.Value{}
	}
	if jsFunction != "" {
		r.Response.Value.Javascript = jsFunction
	}
	if jsPath != "" {
		data, err := utils.ReadFile("", jsPath)
		if err != nil {
			return fmt.Errorf("failed to read JS file: %w", err)
		}
		r.Response.Value.Javascript = string(data)
	}
	return nil
}

func (r *ResponseValue) SetHeader(header map[string]string) {
	r.Response.Value.Headers = header
}

func (r *ResponseValue) SetParams(params []*types.RestParam) {
	r.Response.Value.Params = params
}

func (r *ResponseValue) SetCookies(cookies map[string]string) {
	r.Response.Value.Cookies = cookies
}

func generateMockPostData(namespace, address string, response []*ResponseValue, trafficConfig *types.TrafficConfig) (*types.MockDescription, map[string]map[string]string, error) {
	mockDescription := &types.MockDescription{
		Version: "v1",
		Mock: &types.Mock{
			TrafficConfig: trafficConfig,
			Responses:     []*types.ResponseConfig{},
		},
		Services:  []*types.Service{},
		Endpoints: []*types.Endpoint{},
		Responses: []*types.Response{},
	}

	formMap := make(map[string]map[string]string)
	formMap[mFiles] = make(map[string]string)
	for _, r := range response {
		r.Response.EndpointName = r.EndpointDescription.Endpoints[0].Name
		mockDescription.Responses = append(mockDescription.Responses, r.Response)
		if r.TrafficConfig != nil {
			mockDescription.Mock.Responses = append(mockDescription.Mock.Responses, &types.ResponseConfig{
				ResponseName:  r.Response.Name,
				TrafficConfig: r.TrafficConfig,
			})
		}
		mockDescription.Endpoints = addUniqueEndpoints(r.EndpointDescription, mockDescription.Endpoints)
		mockDescription.Services = addUniqueServices(r.EndpointDescription, mockDescription.Services)
	}
	formMap[mDescription] = make(map[string]string)
	mockDescriptionBytes, err := json.Marshal(mockDescription)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal mock description: %w", err)
	}
	formMap[mDescription][resources] = string(mockDescriptionBytes)
	for _, r := range mockDescription.Endpoints {
		if r.Defined != nil {
			fileContents, err := os.ReadFile(r.Defined.Path)
			if err != nil {
				return nil, nil, err
			}
			formMap[mFiles][r.Defined.Path] = string(fileContents)
		}
	}

	return mockDescription, formMap, nil
}

func addUniqueServices(endpointDesc *types.EndpointDescription, services []*types.Service) []*types.Service {
	serviceName := endpointDesc.Services[0].Name
	var existingService *types.Service
	for _, s := range services {
		if serviceName == s.Name {
			existingService = s
			break
		}
	}

	if existingService == nil {
		endpointDesc.Services[0].Endpoints = []string{endpointDesc.Endpoints[0].Name}
		services = append(services, endpointDesc.Services[0])
	} else {
		endpointExist := false
		for _, endpoint := range endpointDesc.Services[0].Endpoints {
			if endpoint == existingService.Endpoints[0] {
				endpointExist = true
				break
			}
		}
		if !endpointExist {
			existingService.Endpoints = append(existingService.Endpoints, endpointDesc.Endpoints[0].Name)
		}
	}

	return services
}

func addUniqueEndpoints(endpointDesc *types.EndpointDescription, endpoints []*types.Endpoint) []*types.Endpoint {
	endpointName := endpointDesc.Endpoints[0].Name
	var existingEndpoint *types.Endpoint
	for _, e := range endpoints {
		if endpointName == e.Name {
			existingEndpoint = e
			break
		}
	}

	if existingEndpoint == nil {
		endpoints = append(endpoints, endpointDesc.Endpoints[0])
	} else {
		for _, method := range endpointDesc.Endpoints[0].Methods {
			existingEndpoint.Methods = addUniqueMethod(method, existingEndpoint.Methods)
		}
	}
	return endpoints
}

func addUniqueMethod(method *types.Method, methods []*types.Method) []*types.Method {
	methodName := method.Name
	var existingMethod *types.Method
	for _, m := range methods {
		if methodName == m.Name {
			existingMethod = m
			break
		}
	}

	if existingMethod == nil {
		methods = append(methods, method)
	}
	return methods
}
