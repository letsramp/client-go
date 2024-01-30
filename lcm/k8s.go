/*
 * Copyright Skyramp Authors 2024
 */

package lcm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/letsramp/client-go/types"

	"github.com/spf13/viper"
	"helm.sh/helm/v3/pkg/action"
	chartLoader "helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/kube"
	helmRepo "helm.sh/helm/v3/pkg/repo"
	apiAppsv1 "k8s.io/api/apps/v1"
	apiCorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientAppsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	clientCorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	burst = 1000 // max burst of http client requests
	qos   = 1000 // Increase QPS from default 5 to 1000
)

type K8SLCM struct {
	k8sConfig        *restclient.Config
	client           *kubernetes.Clientset
	deploymentClient clientAppsv1.DeploymentInterface
	podClient        clientCorev1.PodInterface

	Namespace     string // Namespace to deploy
	LabelSelector string // labels for filter

	// Results for cache
	Deployments []*apiAppsv1.Deployment
	Services    []*apiCorev1.Service
	ConfigMaps  []*apiCorev1.ConfigMap

	option *LCMOption
}

func NewK8SLCM(configPath, context, namespace string, labelMap map[string]string, option *LCMOption) (*K8SLCM, error) {
	config, err := NewK8sConfig(configPath, context)
	if err != nil {
		return nil, fmt.Errorf("failed to build K8S config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get K8S clientset: %w", err)
	}

	// create namespace if it does not exist
	namespaceClient := clientset.CoreV1().Namespaces()
	if namespace != "" {
		if err = createNamespace(namespaceClient, namespace, labelMap); err != nil {
			return nil, fmt.Errorf("failed to create namespace %s: %w", namespace, err)
		}
	}

	deploymentClient := clientset.AppsV1().Deployments(namespace)
	podClient := clientset.CoreV1().Pods(namespace)

	return &K8SLCM{
		k8sConfig:        config,
		client:           clientset,
		deploymentClient: deploymentClient,
		podClient:        podClient,

		Namespace:     namespace,
		LabelSelector: labels.FormatLabels(labelMap),
		option:        option,
	}, nil
}

type simpleTerminal struct {
	Out      *bytes.Buffer
	ctx      context.Context
	mu       sync.Mutex
	stopping bool
}

func (t *simpleTerminal) Read(p []byte) (int, error) {
	// this will be triggered if skyramp stop is called
	<-t.ctx.Done()

	var s bool
	t.mu.Lock()
	s = t.stopping
	if !s {
		t.stopping = true
		t.mu.Unlock()
		// TODO "exit" is special case for FlexRAN CLI which does not terminate
		// until "exit" is explicitly type (and sigint is blocked)
		return copy(p, []byte{0x03, 0x04, 'e', 'x', 'i', 't', '\n'}), nil
	}
	t.mu.Unlock()
	// wait until it is done
	time.Sleep(1 * time.Second)
	return 0, nil
}

func (t *simpleTerminal) Write(p []byte) (int, error) {
	return t.Out.Write(p)
}

type LCMOption struct {
	DeployIngress bool
}

func (l *K8SLCM) GetPodName(deploymentName string) (string, error) {
	// Create podClient if it does not exist
	if l.podClient == nil {
		l.podClient = l.client.CoreV1().Pods(l.Namespace)
	}

	deployments, err := l.GetDeployments([]string{deploymentName})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment for %s: %w", deploymentName, err)
	}

	if len(deployments) == 0 {
		return "", fmt.Errorf("container is not deployed for deployment, %s", deploymentName)
	}

	deployment := deployments[0]
	matchLabel := deployment.Spec.Selector.MatchLabels[types.SkyrampLabelKey]
	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", types.SkyrampLabelKey, matchLabel),
	}

	podList, err := l.podClient.List(context.TODO(), listOptions)
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("failed to find pods for deployment %s: %w", deploymentName, err)
		}
	}
	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no pod found for deployment %s", deploymentName)
	}
	return podList.Items[0].Name, nil
}

func (l *K8SLCM) GetWorkerPodName() (string, error) {
	var workerPodName string
	var err error
	if workerPodName, err = l.GetPodName(types.WorkerContainerName); err != nil {
		return "", fmt.Errorf("worker is not running")
	}
	return workerPodName, nil
}

func (l *K8SLCM) GetDeployments(deployments []string) ([]*apiAppsv1.Deployment, error) {
	ret := []*apiAppsv1.Deployment{}

	for _, deployment := range deployments {
		result, err := l.deploymentClient.Get(context.TODO(), deployment, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get Deployment: %w", err)
		}
		ret = append(ret, result)
	}

	return ret, nil
}

func (l *K8SLCM) GetClientSet() *kubernetes.Clientset {
	if l == nil {
		return nil
	}

	return l.client
}

// exec sh command over k8s api
func (l *K8SLCM) ExecCommand(ctx context.Context, namespace, podName, containerName, dir, command string) (string, error) {
	restClient := l.client.CoreV1().RESTClient()

	if namespace == "" {
		namespace = l.Namespace
	}

	request := restClient.Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec")
	if containerName != "" {
		request.Param("container", containerName)
	}

	option := &apiCorev1.PodExecOptions{
		Command: []string{"sh", "-c", fmt.Sprintf("cd %s && %s", dir, command)},
		Stdin:   true,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}
	request.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(l.k8sConfig, "POST", request.URL())
	if err != nil {
		return "", fmt.Errorf("failed to exec command %s: %v", command, err)
	}

	newCtx, newCancel := context.WithCancel(ctx)

	terminal := &simpleTerminal{
		Out: new(bytes.Buffer),
		ctx: newCtx,
	}

	f := func() {
		err = exec.Stream(remotecommand.StreamOptions{
			Stdin:  terminal,
			Stdout: terminal,
			Stderr: nil,
			Tty:    true,
		})
		// to make sure terminal is properly closed
		terminal.mu.Lock()
		terminal.stopping = true
		terminal.mu.Unlock()
		newCancel()
	}

	go f()

	// wait until exec is done
	<-newCtx.Done()

	const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

	// remove any ANSI characters
	re := regexp.MustCompile(ansi)
	output := re.ReplaceAllString(terminal.Out.String(), "")

	if err != nil {
		if strings.Contains(err.Error(), "terminated with exit code") {
			return output, fmt.Errorf("%s", err.Error())
		} else if strings.Contains(err.Error(), "container not found") {
			return output, fmt.Errorf("worker pod might be crashed or not running")
		} else {
			return output, fmt.Errorf("failed to get exec results: %w", err)
		}
	}

	return output, nil
}

func NewK8sConfig(configPath, context string) (*restclient.Config, error) {
	// if no context is given, we use current-context of kubeconfig
	if context == "" {
		config, err := clientcmd.BuildConfigFromFlags("", configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build K8S config: %w", err)
		}
		return disableThrottling(config), nil
	}

	// if context is given, we use the context
	configLoadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: configPath,
	}
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: context,
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(configLoadingRules, configOverrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load k8s config: %w", err)
	}

	kubeInsecure := viper.GetBool("kube-insecure")

	if kubeInsecure {
		config.Insecure = kubeInsecure

		// An error occurs if the CA root file is provided and the insecure
		// flag is used
		config.CAFile = ""
		config.CAData = nil
	}

	return disableThrottling(config), nil
}

func disableThrottling(cfg *restclient.Config) *restclient.Config {
	cfg.QPS = qos
	cfg.Burst = burst // Increase Burst from default 10 to 100
	return cfg
}

func createNamespace(client clientCorev1.NamespaceInterface, namespace string, labels map[string]string) error {
	_, err := client.Get(context.TODO(), namespace, metav1.GetOptions{})
	if err == nil {
		// Namespace already exists
		return nil
	}
	if !errors.IsNotFound(err) {
		// Something happened
		return err
	}

	ns := &apiCorev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace,
			Labels: labels,
		},
	}
	_, err = client.Create(context.TODO(), ns, metav1.CreateOptions{})
	return err
}

func InstallHelmChart(
	namespace, deploymentName string,
	helmOptions *types.HelmOptions,
	releaseName, kubeconfigPath, k8sContext string,
	values map[string]interface{},
) error {
	settings := cli.New()
	getters := getter.All(settings)
	k8sLcm, err := NewK8SLCM(kubeconfigPath, k8sContext, namespace, nil, &LCMOption{})
	if err != nil {
		return fmt.Errorf("failed to setup Kubernetes client: %w", err)
	}
	chartURL, err := helmRepo.FindChartInRepoURL(helmOptions.Repo, helmOptions.Chart, helmOptions.Version, "", "", "", getters)
	if err != nil {
		return err
	}
	c := downloader.ChartDownloader{
		Out:     os.Stderr,
		Getters: getters,
	}

	tempPath, err := os.MkdirTemp("", "skyramp-tmp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempPath)

	chartPath, _, err := c.DownloadTo(chartURL, "", tempPath)
	if err != nil {
		return err
	}

	chart, err := chartLoader.Load(chartPath)
	if err != nil {
		return err
	}
	actionConfig := new(action.Configuration)
	if err = actionConfig.Init(kube.GetConfig(kubeconfigPath, "", namespace), namespace, os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {}); err != nil {
		return err
	}

	iCli := action.NewInstall(actionConfig)
	iCli.Namespace = namespace
	iCli.ReleaseName = releaseName
	_, err = iCli.Run(chart, values)
	if err != nil {
		return err
	}

	if deploymentName != "" {
		podName, _ := k8sLcm.GetPodName(deploymentName)
		// Wait for the pod to reach the Running state with 2 mins timeout
		error := wait.PollImmediate(5*time.Second, types.WorkerWaitTime, func() (bool, error) {
			if podName == "" {
				podName, _ = k8sLcm.GetPodName(deploymentName)
				return false, nil
			} else {
				pod, err := k8sLcm.GetClientSet().CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
				if err != nil {
					return false, nil
				}
				if len(pod.Status.ContainerStatuses) > 0 && pod.Status.ContainerStatuses[0].Ready {
					return true, nil
				} else {
					return false, nil
				}
			}
		})
		if error != nil {
			return fmt.Errorf("error occurred while waiting for pod to be ready %v", error)
		}
	}

	return nil
}

func UninstallHelmChart(namespace, releaseName, kubeconfigPath string) error {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(kube.GetConfig(kubeconfigPath, "", namespace), namespace, os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {}); err != nil {
		return err
	}

	iCli := action.NewUninstall(actionConfig)
	_, err := iCli.Run(releaseName)
	if err != nil {
		return err
	}
	return nil
}
