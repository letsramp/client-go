/*
 * Copyright Skyramp Authors 2024
 */
package client_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/letsramp/client-go/client"
	"github.com/letsramp/client-go/lcm"
	"github.com/letsramp/client-go/types"
	"github.com/letsramp/client-go/utils"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/kind/pkg/cluster"
)

const (
	clusterName = "test-cluster"
)

var k8sClient client.Client
var kubeconfig string = filepath.Join(utils.HomeDir(), ".kube/config")

func TestK8sEchoService(t *testing.T) {
	// Create the Kind cluster
	err := createKINDCluster(clusterName, types.KindNodeImage, kubeconfig)
	assert.Equal(t, nil, err)

	namespace := "test-echo-svc"
	k8sContext := fmt.Sprintf("kind-%s", clusterName)
	// Create the Kubernetes client
	k8sClient = client.NewKubernetesClient(
		kubeconfig,
		k8sContext,
		clusterName,
		namespace,
	)
	updateWorker(k8sClient)

	// Install Skyramp worker
	err = k8sClient.InstallWorker()
	assert.Equal(t, nil, err)

	// Install echo service
	err = os.Chdir("../testdata")
	assert.Equal(t, nil, err)

	// building echo image
	err = os.Chdir("../testdata")
	assert.Equal(t, nil, err)
	cmd := exec.Command("docker", "build", "-t", "echo:v0.0", "-f", "dockerfiles/echo/Dockerfile", ".", "--no-cache")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	assert.Equal(t, nil, err)
	client.LoadCustomImageInKind("echo:v0.0", clusterName)
	// deploying echo service with helm using above echo image
	echoRelease := "echo"
	values := make(map[string]interface{})
	values["image"] = map[string]interface{}{
		"repository": "echo",
		"tag":        "v0.0",
	}
	err = lcm.InstallHelmChart(
		namespace,
		"echo",
		&types.HelmOptions{
			Repo:    "https://letsramp.github.io/helm/",
			Chart:   "echo",
			Version: "",
		},
		echoRelease,
		kubeconfig,
		k8sContext,
		values,
	)
	assert.Equal(t, nil, err)

	service := &types.Service{
		Name:         "echo",
		Protocol:     types.Rest,
		ServiceAlias: "echo",
		Port:         12346,
	}
	// Create the REST endpoint
	restEndpoint := client.NewRestEndpoint("echo-get", "/echo/{ msg }", "", "", service)
	// Create the scenario
	scenario := client.NewScenario("echo_test")
	// Create the request
	request := client.NewRequest(
		"echo_get",
		restEndpoint,
		"GET",
		"")
	headers := make(map[string]string)
	headers["key"] = "value"

	request.SetHeader(headers)

	params := make([]*types.RestParam, 0)
	params = append(params, &types.RestParam{
		Name:  "msg",
		In:    "path",
		Value: "ping",
	})

	request.SetParams(params)

	step := scenario.SetRequest(request)
	assert.NotEqual(t, nil, step)
	scenario.SetAssertEqual(step.GetResponseValue("message"), "ping pong")

	status, err := k8sClient.TesterStart([]*client.Scenario{scenario}, "k8s_echo_test", nil, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, types.TesterStatusType("finished"), status.Status)

	err = k8sClient.UninstallWorker()
	assert.Equal(t, nil, err)
	err = lcm.UninstallHelmChart(namespace, echoRelease, kubeconfig)
	assert.Equal(t, nil, err)
}

func TestK8sEchoServiceUsingMocker(t *testing.T) {
	// Create the Kind cluster
	err := createKINDCluster(clusterName, types.KindNodeImage, kubeconfig)
	assert.Equal(t, nil, err)

	namespace := "test-mocked-echo-svc"
	// Create the Kubernetes client
	k8sClient = client.NewKubernetesClient(
		kubeconfig,
		fmt.Sprintf("kind-%s", clusterName),
		clusterName,
		namespace,
	)
	updateWorker(k8sClient)

	err = k8sClient.InstallWorker()
	assert.Equal(t, nil, err)

	service := &types.Service{
		Name:         "echo",
		Protocol:     types.Rest,
		ServiceAlias: "echo",
		Port:         12346,
	}

	restEndpoint := client.NewRestEndpoint("echo-get", "/echo/{ msg }", "", "", service)

	response := client.NewResponseValue(
		"echo_get",
		restEndpoint,
		"GET",
		`{
        	"message": "ping pong"
      	}`,
		&types.TrafficConfig{
			LossPercentage: 0,
			DelayConfig: &types.TrafficDelayConfig{
				MinDelay: 100,
				MaxDelay: 200,
			},
		},
	)
	err = k8sClient.MockerApply([]*client.ResponseValue{response}, nil)
	assert.Equal(t, nil, err)

	scenario := client.NewScenario("echo_test")

	request := client.NewRequest(
		"echo_get",
		restEndpoint,
		"GET",
		"")
	headers := make(map[string]string)
	headers["key"] = "value"

	request.SetHeader(headers)

	params := make([]*types.RestParam, 0)
	params = append(params, &types.RestParam{
		Name:  "msg",
		In:    "path",
		Value: "ping",
	})

	request.SetParams(params)

	step := scenario.SetRequest(request)
	assert.NotEqual(t, nil, step)
	scenario.SetAssertEqual(step.GetResponseValue("message"), "ping pong")
	scenario.SetAssertEqual(step.GetResponseCode(), "200")

	status, err := k8sClient.TesterStart([]*client.Scenario{scenario}, "k8s_mocked_echo_test", nil, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, types.TesterStatusType("finished"), status.Status)

	err = k8sClient.UninstallWorker()
	assert.Equal(t, nil, err)
}

func TestK8sRBAC(t *testing.T) {
	// Create the Kind cluster
	err := createKINDCluster(clusterName, types.KindNodeImage, kubeconfig)
	assert.Equal(t, nil, err)

	namespace := "test-rbac"
	// Create the Kubernetes client
	k8sClient = client.NewKubernetesClient(
		kubeconfig,
		fmt.Sprintf("kind-%s", clusterName),
		clusterName,
		namespace,
	)
	updateWorker(k8sClient)

	err = k8sClient.InstallWorker()
	assert.Equal(t, nil, err)

	service := &types.Service{
		Name:         "rbac",
		Protocol:     types.Rest,
		ServiceAlias: "rbac",
		Port:         60000,
	}

	loginEndpoint := client.NewRestEndpoint("rbac-login", "/auth/v1/login", "", "", service)
	logoutEndpoint := client.NewRestEndpoint("rbac-logout", "/auth/v1/logout", "", "", service)
	loginResponse := client.NewResponseValue(
		"user_login",
		loginEndpoint,
		"POST",
		`{}`,
		nil,
	)
	cookies := make(map[string]string)
	cookies["rsid"] = "rsid"
	cookies["csrftoken"] = "token"

	loginResponse.SetCookies(cookies)

	logoutResponse := client.NewResponseValue(
		"user_logout",
		logoutEndpoint,
		"GET",
		`{}`,
		nil,
	)
	err = k8sClient.MockerApply([]*client.ResponseValue{loginResponse, logoutResponse}, nil)
	assert.Equal(t, nil, err)

	scenario1 := client.NewScenario("login_scenario")

	loginRequest := client.NewRequest(
		"rbac_user_login",
		loginEndpoint,
		"POST",
		`{
         "username": "vars.username",
          "password": "vars.password",
          "organization": "",
          "showPassword": false,
          "isResponseError": false,
          "current_password": "",
          "new_password": "",
          "confirm_password": "",
          "isChangePasswordError": false,
          "change_password": {},
          "redirectToLogin": false,
          "logintype": "",
          "usertype": "internal",
          "loading": false,
          "showRadioBtns": false,
          "redirectUrl": null,
          "isPreLoginError": false,
          "organizationRequired": false
      }`)
	vars := make(map[string]interface{})
	vars["username"] = "default_username"
	vars["password"] = "default_password"

	loginRequest.SetVars(vars)

	scenario1.SetRequest(loginRequest)

	logoutRequest := client.NewRequest(
		"rbac_logout",
		logoutEndpoint,
		"GET",
		`{}`)
	logoutHeader := make(map[string]string)
	logoutHeader["x-csrftoken"] = "vars.token"
	logoutHeader["Referer"] = "globalVars.console_url"
	logoutRequest.SetHeader(logoutHeader)

	logoutVars := make(map[string]interface{})
	logoutVars["token"] = "default_token"
	logoutVars["rsid"] = "default_rsid"
	logoutRequest.SetVars(logoutVars)

	logoutCookie := make(map[string]string)
	logoutCookie["csrftoken"] = "vars.token"
	logoutCookie["rsid"] = "vars.rsid"
	logoutRequest.SetCookies(logoutCookie)

	scenario2 := client.NewScenario("logout_scenario")

	logoutStep := scenario2.SetRequest(logoutRequest)
	scenario2.SetAssertEqual(logoutStep.GetResponseCode(), "200")

	globalVars := map[string]interface{}{
		"console_url":       "console.url",
		"org_admin":         "admin@admin",
		"orgadmin_password": "password",
		"project_id":        "jke1gmw",
		"user_id":           "asdasdasd",
		"username":          "user@user",
		"role_type":         "unknown",
	}
	status, err := k8sClient.TesterStart([]*client.Scenario{scenario1, scenario2}, "k8s_rbac_test", globalVars, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, types.TesterStatusType("finished"), status.Status)

	err = k8sClient.UninstallWorker()
	assert.Equal(t, nil, err)
}

func TestK8sChaining(t *testing.T) {
	// Create the Kind cluster
	err := createKINDCluster(clusterName, types.KindNodeImage, kubeconfig)
	assert.Equal(t, nil, err)

	namespace := "test-chaining"
	// Create the Kubernetes client
	k8sClient = client.NewKubernetesClient(
		kubeconfig,
		fmt.Sprintf("kind-%s", clusterName),
		clusterName,
		namespace,
	)
	updateWorker(k8sClient)

	err = k8sClient.InstallWorker()
	assert.Equal(t, nil, err)

	service := &types.Service{
		Name:         "helloworld",
		Protocol:     types.Grpc,
		ServiceAlias: "helloworld",
		Port:         50051,
	}

	grpcEndpoint := client.NewGrpcEndpoint("helloworld-endpoint", "Greeter", "../testdata/pb/helloworld.proto", "", service)

	response := client.NewResponseValue(
		"mock1",
		grpcEndpoint,
		"SayHello",
		"",
		nil,
	)

	response.SetJSFunction(`function handler(req) {
            return {
              value: {
                message: req.value.name + "temp"
              }
            }
          }`,
		"")
	// mocker apply
	err = k8sClient.MockerApply(
		[]*client.ResponseValue{response},
		&types.TrafficConfig{
			LossPercentage: 0,
			DelayConfig: &types.TrafficDelayConfig{
				MinDelay: 100,
				MaxDelay: 200,
			},
		},
	)
	assert.Equal(t, nil, err)

	scenario := client.NewScenario("scenario1")
	request := client.NewRequest(
		"request1",
		grpcEndpoint,
		"SayHello",
		"")
	request.SetJSFunction(`i = 0
		function handler() {
			i ++
			return {
			value: {
				name: vars.name + i
			}
			}
		}`,
		"")

	request.SetVars(map[string]interface{}{
		"name": "name",
	})
	step1 := scenario.SetRequest(request)
	assert.NotEqual(t, nil, step1)
	scenario.SetAssertEqual(step1.GetResponseValue("message"), "name1temp")

	step2 := scenario.SetRequest(request)
	step2Value := step2.GetResponseValue("message")
	step2.SetValue(map[string]string{
		"name": step2Value})
	scenario.SetAssertEqual(step2Value, "name1temp1temp")

	status, err := k8sClient.TesterStart([]*client.Scenario{scenario}, "k8s_chaining", nil, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, types.TesterStatusType("finished"), status.Status)

	err = k8sClient.UninstallWorker()
	assert.Equal(t, nil, err)
}

func TestCleanupK8sCluster(t *testing.T) {
	err := deleteKINDCluster(clusterName, kubeconfig)
	assert.Equal(t, nil, err)
}

func createKINDCluster(name string, nodeImage string, kubeconfigPath string) error {
	provider := cluster.NewProvider()
	clusters, err := provider.List()
	if err != nil {
		return err
	}
	clusterExists := false
	for _, cluster := range clusters {
		if cluster == name {
			clusterExists = true
			break
		}
	}
	if !clusterExists {
		err = provider.Create(
			name,
			cluster.CreateWithNodeImage(nodeImage),
			cluster.CreateWithKubeconfigPath(kubeconfigPath),
		)
	}
	return err
}

func deleteKINDCluster(name string, kubeconfigPath string) error {
	provider := cluster.NewProvider()
	err := provider.Delete(name, kubeconfigPath)
	return err
}
