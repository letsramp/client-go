/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"github.com/letsramp/client-go/types"
)

// NewGrpcEndpoint creates a new grpc endpoint
// name: the name of the endpoint
// grpcServiceName: the name of the grpc service
// protoFile: the path of the proto file
// address: the address of the service
// serviceOption: the service option
// return: the endpoint description
func NewGrpcEndpoint(name, grpcServiceName string, protoFile, address string, serviceOption *types.Service) *types.EndpointDescription {
	endpointDescription := &types.EndpointDescription{
		Endpoints: []*types.Endpoint{
			{
				Name: name,
				Defined: &types.DefinitionInfo{
					Path: protoFile,
					Name: grpcServiceName,
				},
				ServiceName: serviceOption.Name,
				Methods:     []*types.Method{},
			},
		},
		Services: []*types.Service{
			{
				Name:     serviceOption.Name,
				Port:     serviceOption.Port,
				Protocol: serviceOption.Protocol,
				Secure:   serviceOption.Secure,
				Endpoints: []string{
					name,
				},
			},
		},
	}

	if address != "" {
		endpointDescription.Services[0].Addr = address
	} else {
		endpointDescription.Services[0].ServiceAlias = serviceOption.ServiceAlias
	}

	return endpointDescription
}

// NewRestEndpoint creates a new REST endpoint
// name: the name of the endpoint
// path: the path of the REST endpoint
// openApiFile: the path of the openapi file
// address: the address of the service
// serviceOption: the service option
func NewRestEndpoint(name, path string, openApiFile, address string, serviceOption *types.Service) *types.EndpointDescription {
	endpointDescription := &types.EndpointDescription{
		Endpoints: []*types.Endpoint{
			{
				Name:        name,
				ServiceName: serviceOption.Name,
				Methods:     []*types.Method{},
			},
		},
		Services: []*types.Service{
			{
				Name:     serviceOption.Name,
				Port:     serviceOption.Port,
				Protocol: serviceOption.Protocol,
				Secure:   serviceOption.Secure,
				Endpoints: []string{
					name,
				},
			},
		},
	}
	if openApiFile != "" {
		endpointDescription.Endpoints[0].Defined = &types.DefinitionInfo{
			Path: openApiFile,
		}
	}
	if path != "" {
		endpointDescription.Endpoints[0].RestPath = path
	}

	if address != "" {
		endpointDescription.Services[0].Addr = address
	} else {
		endpointDescription.Services[0].ServiceAlias = serviceOption.ServiceAlias
	}

	return endpointDescription
}

func updateEndpoint(endpointDesc *types.EndpointDescription, methodName string) *types.EndpointDescription {
	isMethodExist := false
	for _, method := range endpointDesc.Endpoints[0].Methods {
		if method.Name == methodName {
			isMethodExist = true
			break
		}
	}

	if !isMethodExist {
		if len(endpointDesc.Services) > 0 && endpointDesc.Services[0].Protocol == types.Grpc {
			endpointDesc.Endpoints[0].Methods = append(endpointDesc.Endpoints[0].Methods, &types.Method{
				Name: methodName,
			})
		} else if len(endpointDesc.Services) > 0 && endpointDesc.Services[0].Protocol == types.Rest {
			endpointDesc.Endpoints[0].Methods = append(endpointDesc.Endpoints[0].Methods, &types.Method{
				Name: methodName,
				Type: types.RestMethodType(methodName),
			})
		}
	}

	return endpointDesc
}
