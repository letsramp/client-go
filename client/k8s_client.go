/*
 * Copyright Skyramp Authors 2024
 */
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

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
}

// NewKubernetesClient creates a new Kubernetes client
func NewKubernetesClient(kubeconfigPath, k8sContext, clusterName, namespace string) *KubernetesClient {
	return &KubernetesClient{
		KubeconfigPath:  kubeconfigPath,
		K8sContext:      k8sContext,
		ClusterName:     clusterName,
		Namespace:       namespace,
		WorkerImageRepo: types.SkyrampWorkerImage,
		WorkerImageTag:  types.SkyrampWorkerImageTag,
	}
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

	return lcm.InstallHelmChart(
		k.Namespace,
		types.WorkerContainerName,
		helmOptions,
		types.WorkerContainerName,
		k.KubeconfigPath,
		k.K8sContext,
		valuesMap,
	)
}

// UninstallWorker uninstalls the Skyramp worker from the Kubernetes cluster
func (k *KubernetesClient) UninstallWorker() error {
	return lcm.UninstallHelmChart(k.Namespace, types.WorkerContainerName, k.KubeconfigPath)
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
	_, formMap, err := generateMockPostData(k.Namespace, "", response, trafficConfig)
	if err != nil {
		return fmt.Errorf("failed to generate mock post data: %w", err)
	}
	k8sLcm, err := lcm.NewK8SLCM(k.KubeconfigPath, k.K8sContext, k.Namespace, nil, &lcm.LCMOption{})
	if err != nil {
		return fmt.Errorf("failed to setup Kubernetes client: %w", err)
	}

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

	_, newFormMap, err := generateTempFilesOnPod(k8sLcm, mockerPodName, formMap)
	if err != nil {
		return err
	}

	curlPayload := generateMultipartFormPayload(newFormMap)

	log.Infof("curlPayload: %+v", curlPayload)

	// Make a request to the worker using the Kubernetes API
	if errString, err := k8sLcm.ExecCommand(context.Background(), "", mockerPodName, "", "/", curlPayload); err != nil {
		return fmt.Errorf("failed to update Skyramp mocker: %s", errString)
	}

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

// TesterStart starts the tests on the Kubernetes cluster
// It takes the scenario, test name, global variables, and global headers as parameters.
// It returns an error if the tests fail to start.
func (k *KubernetesClient) TesterStart(scenario []*Scenario, testName string, globalVars map[string]interface{}, globalHeaders map[string]string) error {
	testRequest, err := generateTestPostRequest(scenario, k.Namespace, testName)
	testRequest.Description.Test.GlobalHeaders = globalHeaders
	testRequest.Description.Test.GlobalVars = globalVars
	if err != nil {
		return fmt.Errorf("failed to generate test post request: %w", err)
	}
	log.Infof("Starting tester")
	requestByte, err := json.Marshal(testRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal test request: %w", err)
	}
	k8sLcm, err := lcm.NewK8SLCM(k.KubeconfigPath, k.K8sContext, k.Namespace, nil, &lcm.LCMOption{})
	if err != nil {
		return fmt.Errorf("failed to setup Kubernetes client: %w", err)
	}
	workerPodName, err := k8sLcm.GetWorkerPodName()
	if err != nil {
		return nil
	}

	postPayload := generateK8sTestPostRequest(requestByte)
	output, err := k8sLcm.ExecCommand(context.Background(), "", workerPodName, "", "/", postPayload)
	if err != nil {
		log.Errorf("failed to start tests: %v", err)
		return fmt.Errorf("failed to start tests: %w", err)
	}

	response, respStatus, err := parseCurlOutput(output)
	if err != nil {
		log.Errorf("failed to parse response: %v", err)
		return fmt.Errorf("failed to parse response from worker: %w", err)
	}
	if respStatus != http.StatusAccepted {
		return fmt.Errorf("worker rejected tests: %s", response.Error)
	}
	responseStatus := k.TestStatus()
	if responseStatus.Status == types.TesterFailed {
		return fmt.Errorf("tester failed: %s", responseStatus.Error)
	}
	return nil
}

// TestStatus returns the status of the tests on the Kubernetes cluster
// It returns the status of the tests.
func (k *KubernetesClient) TestStatus() *types.TestStatusResponse {
	var testStatus *types.TestStatusResponse
	deadline := time.Now().Add(types.WorkerWaitTime)
retry:
	for retries := 0; time.Now().Before(deadline) && retries < maxRetries; retries++ {
		var status int
		var err error
		testStatus, status, err = k.GetTesterStatus()
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

func (k *KubernetesClient) GetTesterStatus() (*types.TestStatusResponse, int, error) {
	statusPayload := generateTestStatusRequest()
	k8sLcm, err := lcm.NewK8SLCM(k.KubeconfigPath, k.K8sContext, k.Namespace, nil, &lcm.LCMOption{})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to setup Kubernetes client: %w", err)
	}
	workerPodName, err := k8sLcm.GetWorkerPodName()
	if err != nil {
		return nil, 0, err
	}

	output, err := k8sLcm.ExecCommand(context.Background(), "", workerPodName, "", "/", statusPayload)
	if err != nil {
		log.Errorf("failed to retrieve test status: %v", err)
		return nil, 0, fmt.Errorf("failed to retrieve test status: %w", err)
	}

	return parseCurlOutput(output)
}

func generateTestStatusRequest() string {
	return fmt.Sprintf("curl -i localhost:%d/%s",
		types.WorkerManagementPort, types.WorkerTestPath)
}

func generateK8sTestPostRequest(payload []byte) string {
	return fmt.Sprintf("curl -XPOST -i -H 'Content-type: application/json' localhost:%d/%s -d '%s'",
		types.WorkerManagementPort, types.WorkerTestPath, string(payload))
}

// Given an lcm object, pod, and formMap, creates a temporary directory with the files.
// The function will return the name of the temporary directory created and
// the form map with the new file paths.
func generateTempFilesOnPod(k8sLcm *lcm.K8SLCM, podName string, formMap map[string]map[string]string) (string, map[string]map[string]string, error) {
	tmpDirCmd := "mktemp -d"

	// Create the temporary directory
	tmpDir, err := k8sLcm.ExecCommand(context.Background(), "", podName, "", "/", tmpDirCmd)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary files: %w", err)
	}
	tmpDir = strings.TrimSpace(tmpDir)

	newFormMap := make(map[string]map[string]string)

	// Iterate through the form map and create all of the temporary files
	for formName, fileNameMap := range formMap {
		newFormMap[formName] = make(map[string]string)
		for fileName, fileContents := range fileNameMap {
			newFileName := fmt.Sprintf("%s/%s", tmpDir, filepath.Base(fileName))

			// For now, verify if the encoding is UTF-8.
			if !utf8.ValidString(fileContents) {
				return "", nil, fmt.Errorf("file, %s, is not UTF-8 encoded", fileName)
			}

			chunks := getFileContentChunks(fileContents, defaultFileChunkByteLength)
			for _, chunk := range chunks {
				// Create a heredoc to write the files to the temporary file location. A heredoc
				// is used to ensure that all "special" characters are dealt with properly.
				// The 'head' command is used to remove the newline that heredocs add at the end.
				createFileCmd := fmt.Sprintf("cat << EOF | head -c -1 >> %s\n%s\nEOF", newFileName, chunk)
				output, err := k8sLcm.ExecCommand(context.Background(), "", podName, "", "/", createFileCmd)
				if err != nil {
					return "", nil, fmt.Errorf("failed to create temporary files (%w): %s", err, output)
				}
			}

			newFormMap[formName][newFileName] = fileContents
		}
	}

	return tmpDir, newFormMap, nil
}

// Writing large files often may fail due to bash's arg length limit.
// This function takes in the content of a file and breaks it up
// into multiple chunks that are smaller in size.
func getFileContentChunks(fileContent string, fileChunkByteLength int) []string {
	chunks := make([]string, 0)

	index := 0
	for index < len(fileContent) {
		boundary := index + fileChunkByteLength
		if len(fileContent) < boundary {
			boundary = len(fileContent)
		}

		chunks = append(chunks, fileContent[index:boundary])
		index = boundary
	}

	return chunks
}

func generateMultipartFormPayload(formMap map[string]map[string]string) string {
	if len(formMap) == 0 {
		return ""
	}

	formArgs := make([]string, 0)
	for formName, fileNameMap := range formMap {
		for fileName := range fileNameMap {
			formArgs = append(formArgs, fmt.Sprintf("-F %s=@%s", formName, fileName))
		}
	}

	formArgsString := strings.Join(formArgs, " ")

	return fmt.Sprintf("curl --fail-with-body -s -XPUT localhost:%d/%s %s", types.WorkerManagementPort, types.WorkerMockConfigPath, formArgsString)
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
