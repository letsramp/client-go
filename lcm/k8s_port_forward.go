/*
 * Copyright Skyramp Authors 2024
 */

package lcm

import (
	"context"
	"fmt"

	"net"
	"net/http"
	"os"
	"os/signal"

	"github.com/pkg/browser"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type K8sPortForwarder struct {
	forwarder *portforward.PortForwarder
	stopChan  chan struct{}
	readyChan <-chan struct{}
	sigChan   chan os.Signal
	done      chan struct{}

	LocalPort int32
}

func (l *K8SLCM) NewK8sPortForward(serviceName, namespace string, dstPort int) (*K8sPortForwarder, error) {
	//find available local port
	localPort, err := findFreePort()
	if err != nil {
		return nil, err
	}

	clientSet := l.GetClientSet()
	restConfig := l.k8sConfig

	// Get the target service
	service, err := clientSet.CoreV1().Services(namespace).Get(context.TODO(), serviceName, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service '%s' in namespace '%s': %v", serviceName, namespace, err)
	}
	logger.Infof("forwarding %s in %s", serviceName, namespace)

	labelSet := labels.Set(service.Spec.Selector)
	listOptions := v1.ListOptions{LabelSelector: labelSet.AsSelector().String()}
	pods, err := clientSet.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to get pods based on selector")
	}

	// Get running Pod
	var runningPod *corev1.Pod
	for _, pod := range pods.Items {
		pod := pod
		if pod.Status.Phase == "Running" {
			runningPod = &pod
			break
		}
	}
	if runningPod == nil {
		return nil, fmt.Errorf("no running %s pod available", serviceName)
	}

	ports := service.Spec.Ports
	if len(ports) == 0 {
		return nil, fmt.Errorf("no ports defined in service %s", serviceName)
	}

	podPort := int32(dstPort)
	// Get pod's tcp port
	if podPort == 0 {
		for _, container := range runningPod.Spec.Containers {
			for _, port := range container.Ports {
				podPort = port.ContainerPort
				break
			}
		}
	}

	// Create a dialer to the pod
	dialer, err := DialerToPod(restConfig, clientSet, runningPod.Name, runningPod.Namespace)
	if err != nil {
		return nil, err
	}
	k8sPortForwarder, err := NewK8sPortForwarder(dialer, localPort, podPort)
	if err != nil {
		return nil, fmt.Errorf("error setting up port forwarding: %s", err)
	}

	logger.Infof("port forwarder for localhost:%d", k8sPortForwarder.LocalPort)

	return k8sPortForwarder, nil
}

func (l *K8SLCM) K8sDashboardPortForward(serviceName, namespace, path string) error {
	forwarder, err := l.NewK8sPortForward(serviceName, namespace, 0)
	if err != nil {
		return err
	}

	// Start port forwarding and open the dashboard in the browser
	err = forwarder.Start(func(*K8sPortForwarder) error {
		url := fmt.Sprintf("http://localhost:%d/%s", forwarder.LocalPort, path)
		fmt.Printf("The dashboard is accessible here: %s\n", url)
		fmt.Print("Port forwarding is started for the dashboard, hit Ctrl+c (or Cmd+c) to stop\n")
		_ = browser.OpenURL(url)

		return nil
	})
	if err != nil {
		return fmt.Errorf("port forwarding failed: %v", err)
	}
	// Wait for the stop signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	<-forwarder.Done()

	return nil
}

func DialerToPod(conf *rest.Config, clientSet kubernetes.Interface, podName string, namespace string) (httpstream.Dialer, error) {
	roundTripper, upgrader, err := spdy.RoundTripperFor(conf)
	if err != nil {
		return nil, fmt.Errorf("error setting up round tripper for port forwarding: %w", err)
	}
	// create a URL to the port-forward endpoint
	serverURL := clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").URL()
	// create a SPDY dialer that will upgrade the connection to the port-forward endpoint
	return spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, serverURL), nil
}

func (pf *K8sPortForwarder) Start(readyFunc func(pf *K8sPortForwarder) error) error {
	// channel is used to receive signals from the OS
	pf.sigChan = make(chan os.Signal, 1)
	// notify the pf.sigChan channel when an interrupt signal is received
	signal.Notify(pf.sigChan, os.Interrupt)
	errChan := make(chan error, 1)
	go func() {
		//port forwarder will block until it receives a stop signal
		err := pf.forwarder.ForwardPorts()
		errChan <- err
	}()
	go func() {
		// wait for the ready signal or an error
		<-pf.sigChan
		pf.Stop()
	}()
	select {
	case <-pf.readyChan:
		// TODO double check on resource cleanup
		go func() {
			_ = readyFunc(pf)
		}()
		return nil
	case err := <-errChan:
		return fmt.Errorf("error during port forwarding: %w", err)
	}
}

func NewK8sPortForwarder(dialer httpstream.Dialer, localPort, dstPort int32) (*K8sPortForwarder, error) {
	portSpec := fmt.Sprintf("%d:%d", localPort, dstPort)
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	forwarder, err := portforward.New(dialer, []string{portSpec}, stopChan, readyChan, nil, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("error setting up port forwarding: %w", err)
	}

	return &K8sPortForwarder{
		forwarder: forwarder,
		stopChan:  stopChan,
		readyChan: readyChan,
		done:      make(chan struct{}),
		LocalPort: localPort,
	}, nil
}

func (pf *K8sPortForwarder) Stop() {
	defer close(pf.done)
	signal.Stop(pf.sigChan)
	if pf.stopChan != nil {
		close(pf.stopChan)
	}
}

func (pf *K8sPortForwarder) Done() <-chan struct{} {
	return pf.done
}

func findFreePort() (int32, error) {
	//create listener for tcp on random port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	//if the listener is successful, get the port
	port := listener.Addr().(*net.TCPAddr).Port
	//convert listener to file so we can close it later
	file, err := listener.(*net.TCPListener).File()
	if err != nil {
		return 0, err
	}
	//close the listener
	err = listener.Close()
	if err != nil {
		return 0, err
	}

	err = file.Close()
	if err != nil {
		return 0, err
	}

	return int32(port), nil
}
