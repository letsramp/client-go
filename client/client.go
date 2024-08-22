/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"fmt"

	"github.com/letsramp/client-go/types"
)

// Client represents an interface for interacting with Skyramp.
type Client interface {
	// InstallWorker installs the worker in the client's environment.
	// It returns an error if the worker installation fails.
	InstallWorker() error

	// UninstallWorker uninstalls the worker in the client's environment.
	// It returns an error if the worker uninstallation fails.
	UninstallWorker() error

	// TesterStart starts the test in the client's environment.
	// It takes the test scenario, test name, global variables, and global headers as parameters.
	// It returns an error if the test fails to start.
	TesterStart(scenario []*Scenario, testName string, globalVars map[string]interface{}, globalHeaders map[string]string) (*types.TestStatusResponse, error)

	// MockerApply applies the mock in the client's environment.
	// It takes the response values and traffic configuration as parameters.
	// It returns an error if the mock fails to apply.
	MockerApply(response []*ResponseValue, trafficConfig *types.TrafficConfig) error

	// TestStatus returns the status of the test in the client's environment.
	// It returns the test status response.
	TestStatus(testId string) *types.TestStatusResponse

	// SetWorkerImage overrides the default worker image.
	// It takes the image and tag as parameters.
	SetWorkerImage(image, tag string)
}

func NewClient(address, namespace, kubeconfigPath, kubeContext, clusterName string) (Client, error) {
	if address != "" && (namespace != "" || kubeconfigPath != "" || kubeContext != "" || clusterName != "") {
		return nil, fmt.Errorf("address cannot be used with k8s related parameters")
	}
	if address == "" && namespace == "" && kubeconfigPath == "" && kubeContext == "" && clusterName == "" {
		return nil, fmt.Errorf("either address or one of k8s related parameters should be given")
	}

	if address != "" {
		return NewDockerClient(address, "")
	}

	return NewKubernetesClient(kubeconfigPath, kubeContext, clusterName, namespace)
}
