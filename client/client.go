/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/letsramp/client-go/types"
)

const DefaultDeployTime = 3 * time.Second

type StringList []string

func (s StringList) String() string {
	return strings.Join(s, ",")
}

func (s *StringList) Set(value string) error {
	*s = strings.Split(value, ",")
	return nil
}

type Config struct {
	WorkerAddress string
	ScenarioName  string
	TestName      string
	K8sConfigPath string
	K8sContext    string
	ClusterName   string
	Namespace     string
	WorkerPort    int
	DeployWorker  bool
	GlobalVars    map[string]interface{}
}

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

	// GetTestConfig returns the test configuration.
	GetTestConfig() *TestConfig

	// Cleanup uninstalls the worker in the client's environment.
	Cleanup()
}

// TestConfig represents the configuration for a test.
type TestConfig struct {
	// ScenarioName is the name of the scenario to be executed.
	ScenarioName string
	// TestName is the name of the test to be executed.
	TestName string
	// GlobalVars are the global variables to be used in the test.
	GlobalVars map[string]interface{}
	// DeployWorker is a flag to indicate if the worker should be installed.
	DeployWorker bool
}

var (
	workerAddress  string
	namespace      string
	k8sConfigPath  string
	k8sContext     string
	clusterName    string
	scenarioName   string
	testName       string
	deployWorker   bool
	globalVarFlags StringList
)

// Configuration
func init() {
	flag.StringVar(&workerAddress, "address", "", "worker address")
	flag.StringVar(&namespace, "namespace", "", "namespace of worker")
	flag.StringVar(&k8sConfigPath, "kubeconfig-path", "", "kubeconfig path")
	flag.StringVar(&k8sContext, "kubeconfig-context", "", "kubeconfig context")
	flag.StringVar(&clusterName, "cluster-name", "", "Skyramp cluster name")
	flag.StringVar(&scenarioName, "scenario-name", "testScenario", "scenario name")
	flag.StringVar(&testName, "test-name", "testName", "test name")
	flag.BoolVar(&deployWorker, "deploy-worker", false, "deploy Skyramp worker")
	flag.Var(&globalVarFlags, "global-vars", "global variables")
}

func NewClient(config *Config) (Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}
	workerAddress = config.WorkerAddress
	namespace = config.Namespace
	k8sConfigPath = config.K8sConfigPath
	k8sContext = config.K8sContext
	clusterName = config.ClusterName
	scenarioName = config.ScenarioName
	testName = config.TestName
	deployWorker = config.DeployWorker

	testConfig := &TestConfig{
		ScenarioName: scenarioName,
		TestName:     testName,
		DeployWorker: deployWorker,
		GlobalVars:   config.GlobalVars,
	}
	if workerAddress != "" && (namespace != "" || k8sConfigPath != "" || k8sContext != "" || clusterName != "") {
		return nil, fmt.Errorf("address cannot be used with k8s related parameters")
	}
	if workerAddress == "" && namespace == "" && k8sConfigPath == "" && k8sContext == "" && clusterName == "" {
		return nil, fmt.Errorf("either address or one of k8s related parameters should be given")
	}
	var skyrampClient Client
	var err error

	if workerAddress != "" {
		skyrampClient, err = NewDockerClient(workerAddress, "", testConfig)
	} else {
		skyrampClient, err = NewKubernetesClient(k8sConfigPath, k8sContext, clusterName, namespace, testConfig)
	}
	if err != nil {
		return nil, err
	}
	if deployWorker {
		err = skyrampClient.InstallWorker()
		if err != nil {
			return nil, err
		}
		time.Sleep(DefaultDeployTime)
	}
	return skyrampClient, nil
}

// ParseArgs parses the command line arguments and returns the configuration.
func ParseArgs() (*Config, error) {
	flag.Parse()
	globalVars := make(map[string]interface{})
	if len(globalVarFlags) != 0 {
		for _, flag := range globalVarFlags {
			fields := strings.Split(flag, "=")
			if len(fields) != 2 {
				return nil, fmt.Errorf("failed to parse globalVar flag. expecting  key=value[,key=value]")
			}
			globalVars[fields[0]] = fields[1]
		}
	}
	return &Config{
		WorkerAddress: workerAddress,
		Namespace:     namespace,
		TestName:      testName,
		ScenarioName:  scenarioName,
		GlobalVars:    globalVars,
		DeployWorker:  deployWorker,
		K8sConfigPath: k8sConfigPath,
		K8sContext:    k8sContext,
		ClusterName:   clusterName,
	}, nil
}
