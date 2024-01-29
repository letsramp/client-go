/*
 * Copyright Skyramp Authors 2024
 */
package types

type EndpointDescription struct {
	Version   string      `json:"version" yaml:"version"`
	Endpoints []*Endpoint `json:"endpoints" yaml:"endpoints"`
	Services  []*Service  `json:"services,omitempty" yaml:"services,omitempty"`
}

// High level object to parse any mock descriptions
type MockDescription struct {
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	// This contains mock definition that include mock responses, traffic configs, proxies etc.
	Mock *Mock `json:"mock,omitempty" yaml:"mock,omitempty"`
	// Service is any endpoint definitions including K8s Svc, FQDN, IP
	Services []*Service `json:"services,omitempty" yaml:"services,omitempty"`
	// This is a wrapper gRPC service or REST path
	Endpoints []*Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	// responseValue is json blob or javascript to be used to generate mock values
	Responses []*Response `json:"responses,omitempty" yaml:"responses,omitempty"`
}

// High level object to parse any test descriptions
type TestDescription struct {
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	// test definitions
	Test *Test `json:"test,omitempty" yaml:"test,omitempty"`
	// test scenario definitions
	Scenarios []*TestScenario `json:"scenarios,omitempty" yaml:"scenarios,omitempty"`
	// API request definitions (REST, gRPC, Thrift)
	Requests []*TestRequest `json:"requests,omitempty" yaml:"requests,omitempty"`
	// Shell command definitions
	Commands []*TestCommand `json:"commands,omitempty" yaml:"commands,omitempty"`
	// Service is any endpoint definitions including K8s Svc, FQDN, IP
	Services []*Service `json:"services,omitempty" yaml:"services,omitempty"`
	// Endpoint definitions (for target without container descriptions)
	Endpoints []*Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
}
