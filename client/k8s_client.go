/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/letsramp/client-go/lcm"
	"github.com/letsramp/client-go/types"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
)

const (
	// ~ 32KByte
	defaultFileChunkByteLength = 1 << 15
)

// KubernetesClient is a client for interacting with Kubernetes
type KubernetesClient struct {
	ClientBase
	// KubeconfigPath is the path to the kubeconfig file
	KubeconfigPath string
	// K8sContext is the context to use
	K8sContext string
	// ClusterName is the name of the cluster
	ClusterName string
	// Namespace is the namespace to use
	Namespace string
	// WorkerImageRepo is the repository of the worker image
	WorkerImageRepo string
	// WorkerImageTag is the tag of the worker image
	WorkerImageTag string

	// TestConfig is the test configuration for the client.
	TestConfig *TestConfig

	// http connection of portforward
	PortForwarder *lcm.K8sPortForwarder
	// channel to control port forwarder
	waitChan chan struct{}
}

// NewKubernetesClient creates a new Kubernetes client
func NewKubernetesClient(kubeconfigPath, k8sContext, clusterName, namespace string, testConfig *TestConfig) (*KubernetesClient, error) {
	if testConfig == nil {
		return nil, fmt.Errorf("testConfig is required for KubernetesClient")
	}
	ret := &KubernetesClient{
		KubeconfigPath:  kubeconfigPath,
		K8sContext:      k8sContext,
		ClusterName:     clusterName,
		Namespace:       namespace,
		WorkerImageRepo: types.SkyrampWorkerImage,
		WorkerImageTag:  types.SkyrampWorkerImageTag,
		TestConfig:      testConfig,
	}

	if !testConfig.DeployWorker {
		_ = ret.StartPortForward()
	} else {
		err := ret.InstallWorker()
		if err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (k *KubernetesClient) StartPortForward() error {
	k8sLcm, err := lcm.NewK8SLCM(k.KubeconfigPath, k.K8sContext, k.Namespace, nil, &lcm.LCMOption{})
	if err != nil {
		return fmt.Errorf("failed to setup Kubernetes client: %w", err)
	}

	k.waitChan = make(chan struct{})
	// k8s commands will be over portforwarder
	portForwarder, err := k8sLcm.NewK8sPortForward(types.WorkerSVCName, k.Namespace, types.WorkerManagementPort)
	if err != nil {
		return fmt.Errorf("failed to create k8s port forwarder: %w", err)
	}

	err = portForwarder.Start(func(*lcm.K8sPortForwarder) error {
		<-k.waitChan
		return nil
	})
	if err != nil {
		return err
	}

	k.PortForwarder = portForwarder
	k.address = fmt.Sprintf("localhost:%d", portForwarder.LocalPort)

	return nil
}

// InstallWorker installs the Skyramp worker on the Kubernetes cluster
func (k *KubernetesClient) InstallWorker() error {
	valuesMap := make(map[string]interface{})
	valuesMap["rbac"] = true

	img := fmt.Sprintf("%s:%s", k.WorkerImageRepo, k.WorkerImageTag)
	publicImg := fmt.Sprintf("%s:%s", types.SkyrampWorkerImage, types.SkyrampWorkerImageTag)
	// check if image is latest public image
	if img != publicImg {
		// load custom image in kind cluster if it is not public image
		if k.WorkerImageRepo != types.SkyrampWorkerImage && strings.HasPrefix(k.K8sContext, "kind-") {
			err := LoadCustomImageInKind(img, k.ClusterName)
			if err != nil {
				return fmt.Errorf("failed to load custom image: %w", err)
			}
		}
		valuesMap["image"] = map[string]string{
			"repository": k.WorkerImageRepo,
			"tag":        k.WorkerImageTag,
		}
	}

	helmOptions := &types.HelmOptions{
		Repo:    "https://letsramp.github.io/helm/",
		Chart:   "worker",
		Version: "",
	}

	err := lcm.InstallHelmChart(
		k.Namespace,
		types.WorkerContainerName,
		helmOptions,
		types.WorkerContainerName,
		k.KubeconfigPath,
		k.K8sContext,
		valuesMap,
	)
	if err != nil {
		return err
	}

	return k.StartPortForward()
}

// UninstallWorker uninstalls the Skyramp worker from the Kubernetes cluster
func (k *KubernetesClient) UninstallWorker() error {
	return lcm.UninstallHelmChart(k.Namespace, types.WorkerContainerName, k.KubeconfigPath)
}

func (k *KubernetesClient) Cleanup() {
	if k.TestConfig.DeployWorker {
		_ = k.UninstallWorker()
		time.Sleep(DefaultDeployTime)
	}
}

// SetWorkerImage used for using custom worker image
func (k *KubernetesClient) SetWorkerImage(image, tag string) {
	k.WorkerImageRepo = image
	k.WorkerImageTag = tag
}

// MockerApply applies the given mocks to the Kubernetes cluster
// It takes the response values and traffic configuration as parameters.
// It returns an error if the mock fails to apply.
func (k *KubernetesClient) MockerApply(response []*ResponseValue, trafficConfig *types.TrafficConfig) error {
	_, err := k.MockerApplyCommon(response, trafficConfig)
	if err != nil {
		return err
	}

	k8sLcm, err := lcm.NewK8SLCM(k.KubeconfigPath, k.K8sContext, k.Namespace, nil, &lcm.LCMOption{})

	var mockerPodName string
	if mockerPodName, err = k8sLcm.GetPodName(types.WorkerContainerName); err != nil {
		return fmt.Errorf("failed to find worker - ensure the Skyramp worker has been installed and the correct namespace is specified")
	}

	// Set up the watch options with timeout
	options := v1.ListOptions{
		LabelSelector: types.SkyrampLabelKey + "=" + types.WorkerContainerName,
		Watch:         true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	watchChan, err := k8sLcm.GetClientSet().CoreV1().Pods(k.Namespace).Watch(ctx, options)
	if err != nil {
		log.Errorf("Failed to create watch: %v", err)
	}
	defer watchChan.Stop()

	podRestarted := false
	// Loop through the watch events and check for terminated state for worker pod
	for event := range watchChan.ResultChan() {
		if event.Type == watch.Modified {
			pod, ok := event.Object.(*corev1.Pod)
			if ok && pod.Status.ContainerStatuses[0].State.Terminated != nil {
				podRestarted = true
				break
			}
		}
	}
	if podRestarted {
		// Wait for the pod to reach the Running state with 2 mins timeout
		error := wait.PollImmediate(5*time.Second, types.WorkerWaitTime, func() (bool, error) {
			pod, err := k8sLcm.GetClientSet().CoreV1().Pods(k.Namespace).Get(context.Background(), mockerPodName, v1.GetOptions{})
			if err != nil {
				return false, err
			}
			return pod.Status.ContainerStatuses[0].Ready, nil
		})
		if error != nil {
			log.Errorf("error occurred while applying mocks %v", error)
			return nil
		}
	}

	return nil
}

func (k *KubernetesClient) GetTestConfig() *TestConfig {
	return k.TestConfig
}

func LoadCustomImageInKind(img string, clusterName string) error {
	cmd := exec.Command("kind", "load", "docker-image", img, "--name", clusterName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to load custom image: %w", err)
	}
	return nil
}
