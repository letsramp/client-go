/*
 * Copyright Skyramp Authors 2024
 */
package lcm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/letsramp/client-go/types"
	dockerContainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	docker "github.com/moby/moby/client"
	log "github.com/sirupsen/logrus"
)

var logger = log.WithField("module", "client")

type DockerLCM struct {
	// docker client
	client *docker.Client
	// docker container object
	Container *dockerContainer.InspectResponse
	// docker network ID that the work is in
	NetworkID string
	// docker network name
	NetworkName string
}

var (
	DockerEngineAPIVersion = "1.44"
)

// Docker's own OSType values, as reported by /info.
const (
	dockerOSTypeLinux   = "linux"
	dockerOSTypeWindows = "windows"

	// windowsWorkerConfigDir is the Windows form of
	// types.DefaultWorkerConfigDirPath, created by the image via WORKDIR.
	windowsWorkerConfigDir = `C:\etc\skyramp`
)

// daemonOSType reports which OS the daemon runs containers for, which decides
// in-container paths. Not runtime.GOOS: Docker Desktop on Windows in
// Linux-container mode still runs linux containers. Falls back to linux when
// /info is unreachable.
//
// Duplicated from pkg/lcm.DaemonOSType because client-go is an independent
// module; keep the two in sync.
func daemonOSType(ctx context.Context, cli *docker.Client) string {
	res, err := cli.Info(ctx, docker.InfoOptions{})
	if err != nil {
		logger.Warnf("could not read daemon OSType (%v); assuming %s", err, dockerOSTypeLinux)
		return dockerOSTypeLinux
	}
	if strings.EqualFold(res.Info.OSType, dockerOSTypeWindows) {
		return dockerOSTypeWindows
	}
	return dockerOSTypeLinux
}

// workerConfigDirForOSType is the in-container mount target for the worker's
// resource directory. types.DefaultWorkerConfigDirPath is a Unix path and is
// also used for the Kubernetes mount, so the OS-aware form lives here.
func workerConfigDirForOSType(osType string) string {
	if osType == dockerOSTypeWindows {
		return windowsWorkerConfigDir
	}
	return types.DefaultWorkerConfigDirPath
}

// generic docker LCM to communicate with docker API
func NewDockerLCM() (*DockerLCM, error) {
	client, err := docker.New(docker.WithAPIVersion(DockerEngineAPIVersion), docker.FromEnv)
	if err != nil {
		logger.Errorf("failed to create docker client: %v", err)
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerLCM{
		client: client,
	}, nil
}

// exposing new ports requires recreating container
// this applies to standalone container, that does not belong to docker network
func (d *DockerLCM) RestartWithNewPorts(container *dockerContainer.InspectResponse, ports []int) error {
	ctx := context.Background()

	// get relevant fields from original container
	config := container.Config
	networkSettings := container.NetworkSettings
	hostConfig := container.HostConfig
	name := container.Name

	// add new ports
	if config.ExposedPorts == nil {
		config.ExposedPorts = network.PortSet{}
	}

	for _, p := range ports {
		newPort, ok := network.PortFrom(uint16(p), network.TCP)
		if !ok {
			return fmt.Errorf("invalid port %d", p)
		}
		config.ExposedPorts[newPort] = struct{}{}
		hostConfig.PortBindings[newPort] = []network.PortBinding{
			{
				HostIP:   netip.MustParseAddr("127.0.0.1"),
				HostPort: strconv.Itoa(p),
			},
		}
	}

	// stop and remove existing container
	logger.Infof("attempting to kill container %s", name)
	if _, err := d.client.ContainerRemove(ctx, container.ID, docker.ContainerRemoveOptions{Force: true}); err != nil {
		logger.Errorf("failed to remove container %s: %v", name, err)
		return fmt.Errorf("failed to remove container %s: %v", name, err)
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: networkSettings.Networks,
	}

	// create and run it in background
	resp, err := d.client.ContainerCreate(ctx, docker.ContainerCreateOptions{
		Config:           config,
		HostConfig:       hostConfig,
		NetworkingConfig: networkConfig,
		Name:             name,
	})
	if err != nil {
		logger.Errorf("failed to create a container %s: %v", name, err)
		return fmt.Errorf("failed to create a container %s: %w", name, err)
	}

	if _, err := d.client.ContainerStart(ctx, resp.ID, docker.ContainerStartOptions{}); err != nil {
		logger.Errorf("failed to start container %s: %v", name, err)
		return fmt.Errorf("failed to start container %s: %v", name, err)
	}

	logger.Infof("successfully restarted %s\n", container.Name)
	return nil
}

// Helper method that will bring up skyramp worker in a docker network
func RunSkyrampWorkerInDockerNetwork(image, tag string, hostPort int, targetNetworkName string) error {
	containerName := types.WorkerContainerName
	containerExposedPort := fmt.Sprintf("%d/tcp", types.WorkerManagementPort)

	dockerLcm, err := NewDockerLCM()
	if err != nil {
		return fmt.Errorf("error occurred while creating docker client: %v", err)
	}

	// Create a Docker volume
	vol, err := dockerLcm.client.VolumeCreate(context.Background(), docker.VolumeCreateOptions{
		Name: containerName,
	})

	if err != nil {
		return fmt.Errorf("error occurred while creating volume: %v", err)
	}
	imagePath := fmt.Sprintf("%s:%s", image, tag)
	_, err = dockerLcm.client.ImageInspect(context.Background(), imagePath)
	if err != nil {
		out, err := dockerLcm.client.ImagePull(context.Background(), imagePath, docker.ImagePullOptions{})
		if err != nil {
			return fmt.Errorf("error occurred while pulling image %s: %v", imagePath, err)
		}
		reader := bufio.NewReader(out)
		if _, err = io.ReadAll(reader); err != nil {
			return fmt.Errorf("error reading image data at pull: %v", err)
		}
		out.Close()
	}
	workerConfigDir := workerConfigDirForOSType(daemonOSType(context.Background(), dockerLcm.client))

	workerContainer, err := dockerLcm.client.ContainerCreate(context.Background(), docker.ContainerCreateOptions{
		Config: &dockerContainer.Config{
			Image: imagePath,
			Volumes: map[string]struct{}{
				workerConfigDir: {},
			},
			ExposedPorts: network.PortSet{
				network.MustParsePort(containerExposedPort): {},
			},
		},
		HostConfig: &dockerContainer.HostConfig{
			Binds: []string{
				fmt.Sprintf("%s:%s:rw", vol.Volume.Name, workerConfigDir),
			},
			RestartPolicy: dockerContainer.RestartPolicy{
				Name: "always",
			},
			PortBindings: network.PortMap{
				network.MustParsePort(containerExposedPort): {{HostPort: fmt.Sprintf("%d", hostPort)}},
			},
		},
		NetworkingConfig: &network.NetworkingConfig{},
		Name:             containerName,
	})
	if err != nil {
		return fmt.Errorf("error occurred while creating container %s: %v", containerName, err)
	}

	_, err = dockerLcm.client.ContainerStart(context.Background(), workerContainer.ID, docker.ContainerStartOptions{})
	if err != nil {
		return fmt.Errorf("error occurred while starting container %s: %v", containerName, err)
	}

	containerInfo, err := inspectContainer(context.Background(), dockerLcm.client, workerContainer.ID)
	if err != nil {
		return fmt.Errorf("error occurred while inspecting container %s: %v", containerName, err)
	}
	dockerLcm.Container = &containerInfo

	if targetNetworkName != "" {
		targetNetwork, err := dockerLcm.FindNetworkByName(targetNetworkName)
		if err != nil {
			return fmt.Errorf("error occurred while getting network name for  %s: %v", targetNetworkName, err)
		}

		var workerNetwork *network.EndpointSettings
		for _, n := range dockerLcm.Container.NetworkSettings.Networks {
			workerNetwork = n
			break
		}

		_, err = dockerLcm.client.ContainerStop(context.Background(), dockerLcm.Container.ID, docker.ContainerStopOptions{})
		if err != nil {
			return fmt.Errorf("error occurred while stoping container %s: %v", containerName, err)
		}
		_, err = dockerLcm.client.NetworkDisconnect(context.Background(), workerNetwork.NetworkID, docker.NetworkDisconnectOptions{Container: dockerLcm.Container.ID, Force: true})
		if err != nil {
			return fmt.Errorf("error occurred while disconnecting network for %s container: %v", containerName, err)
		}
		_, err = dockerLcm.client.NetworkConnect(context.Background(), targetNetwork.ID, docker.NetworkConnectOptions{Container: dockerLcm.Container.ID, EndpointConfig: &network.EndpointSettings{}})
		if err != nil {
			return fmt.Errorf("error occurred while attaching container %s to %s: %v", containerName, targetNetwork.ID, err)
		}
		_, err = dockerLcm.client.ContainerStart(context.Background(), dockerLcm.Container.ID, docker.ContainerStartOptions{})
		if err != nil {
			return fmt.Errorf("error occurred while starting container %s: %v", containerName, err)
		}
	}

	// wait for container to be ready
	err = waitForContainerStarted(dockerLcm.client, context.Background(), workerContainer.ID, hostPort)
	if err != nil {
		return fmt.Errorf("error occurred while waiting for container %s to start: %v", containerName, err)
	}
	return nil
}

func waitForContainerStarted(client *docker.Client, ctx context.Context, containerID string, hostPort int) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil // Context done, no error
		case <-ticker.C:
			// Check container status
			status, err := inspectContainer(ctx, client, containerID)
			if err != nil {
				return fmt.Errorf("failed to inspect container %s: %w", containerID, err)
			}

			if status.State.Running {
				if isContainerReady(status, readinessHost(client.DaemonHost()), hostPort) {
					return nil
				}
			}
		}
	}
}

// readyzProbeClient bounds one /readyz probe. http.DefaultClient has no timeout, so a
// daemon that blackholes packets would hold the polling goroutine until the OS TCP
// timeout, well past the caller's own deadline.
//
// Explicit transport with no Proxy: the default one honours HTTP_PROXY, and this probe
// targets a port published directly on the daemon's host. A proxy would either
// black-hole it or answer 200 itself and be read as readiness.
//
// client-go is its own Go module and cannot import pkg/lcm, so this mirrors
// pkg/lcm/docker_lcm.go's probe rather than sharing it. Keep the two in step: they answer
// the same question about the same worker image.
var readyzProbeClient = &http.Client{
	Timeout:   5 * time.Second,
	Transport: &http.Transport{Proxy: nil},
}

// isContainerReady prefers the image's own healthcheck and falls back to the worker's
// /readyz on its published port when the image declares none.
//
// The fallback is what makes the lean Windows worker deployable through this client: its
// base image ships neither PowerShell nor curl, so Dockerfile.windows has no HEALTHCHECK,
// and "healthy or nothing" left Health nil forever and this loop waiting until the caller
// gave up.
//
// host is where that published port is reachable, which is not always localhost: this
// package builds its client with docker.FromEnv, so a DOCKER_HOST naming another machine
// publishes the port there and a probe of 127.0.0.1 asks the wrong host.
func isContainerReady(status dockerContainer.InspectResponse, host string, hostPort int) bool {
	if status.State.Health != nil {
		return status.State.Health.Status == "healthy"
	}
	if hostPort <= 0 {
		return false
	}
	if host == "" {
		host = "127.0.0.1"
	}
	resp, err := readyzProbeClient.Get("http://" + net.JoinHostPort(host, strconv.Itoa(hostPort)) + "/readyz")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// readinessHost is the host a container's published port is reachable on for the daemon
// at daemonHost. Only a daemon addressed over TCP is somewhere else; a unix socket or a
// named pipe is this machine, and an unparseable address degrades to localhost so the
// connection attempt reports it.
func readinessHost(daemonHost string) string {
	if daemonHost == "" {
		return "127.0.0.1"
	}
	u, err := docker.ParseHostURL(daemonHost)
	if err != nil {
		return "127.0.0.1"
	}
	switch u.Scheme {
	case "tcp", "http", "https":
		if h, _, err := net.SplitHostPort(u.Host); err == nil && h != "" {
			return h
		}
		if u.Host != "" {
			return u.Host
		}
	}
	return "127.0.0.1"
}

// Helper method that will bring down skyramp worker in a docker network
func RemoveSkyrampWorkerInDocker(containerName string) error {
	dockerLcm, err := NewDockerLCM()
	if err != nil {
		return fmt.Errorf("error occurred while creating docker client: %v", err)
	}

	workerContainer, err := findContainerByName(containerName)
	if err != nil {
		return fmt.Errorf("error occurred while finding container %s: %v", containerName, err)
	}

	// delete container
	_, _ = dockerLcm.client.ContainerRemove(context.Background(), workerContainer.ID, docker.ContainerRemoveOptions{Force: true})

	// delete docker volume
	_, err = dockerLcm.client.VolumeRemove(context.Background(), containerName, docker.VolumeRemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("error occurred while removing volume %s: %v", containerName, err)
	}

	return nil
}

func findContainerByName(containerName string) (*dockerContainer.Summary, error) {
	dockerLcm, err := NewDockerLCM()
	if err != nil {
		return nil, err
	}

	containers, err := dockerLcm.client.ContainerList(context.Background(), docker.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	for _, c := range containers.Items {
		if c.Names[0] == "/"+containerName {
			return &c, nil
		}
	}

	return nil, fmt.Errorf("container %s not found", containerName)
}

func (d *DockerLCM) FindNetworkByName(networkName string) (*network.Inspect, error) {
	networks, err := d.client.NetworkList(context.Background(), docker.NetworkListOptions{})
	if err != nil {
		return nil, err
	}

	var networkID string
	for _, n := range networks.Items {
		if n.Name == networkName {
			networkID = n.ID
			break
		}
	}

	if networkID == "" {
		return nil, fmt.Errorf("failed to find network %s", networkID)
	}

	res, err := d.client.NetworkInspect(context.Background(), networkID, docker.NetworkInspectOptions{})
	if err != nil {
		return nil, err
	}

	return &res.Network, nil
}

// find container in docker using address and IP
// there could be some edges cases that this does not properly handle
func (d *DockerLCM) FindContainerByBoundAddress(address string) (*dockerContainer.InspectResponse, error) {
	containers, err := d.client.ContainerList(context.Background(), docker.ContainerListOptions{})
	if err != nil {
		return nil, err
	}

	addrPort, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", address, err)
	}

	for _, c := range containers.Items {
		container, err := inspectContainer(context.Background(), d.client, c.ID)
		if err != nil {
			return nil, err
		}

		portBindings := container.HostConfig.PortBindings
		for _, bindings := range portBindings {
			for _, b := range bindings {
				if b.HostPort == strconv.Itoa(addrPort.Port) && isIPSame(addrString(b.HostIP), addrPort.IP.String()) {
					return &container, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("container does not exist")
}

func isIPSame(ip1, ip2 string) bool {
	// special edge case for "0.0.0.0"
	if ip1 == "" || ip2 == "" {
		return true
	}

	n1 := net.ParseIP(ip1)
	n2 := net.ParseIP(ip2)

	return n1.Equal(n2)
}

func IsInDefaultBridgeNetwork(container *dockerContainer.InspectResponse) bool {
	networkName, _ := getNetworkEndpointOfContainer(container)
	return networkName == "bridge"
}

// this assumes container belongs to a single network only
func getNetworkEndpointOfContainer(container *dockerContainer.InspectResponse) (string, *network.EndpointSettings) {
	if container.NetworkSettings == nil {
		return "", nil
	}

	for name, e := range container.NetworkSettings.Networks {
		return name, e
	}

	return "", nil
}

func inspectContainer(ctx context.Context, cli *docker.Client, containerID string) (dockerContainer.InspectResponse, error) {
	res, err := cli.ContainerInspect(ctx, containerID, docker.ContainerInspectOptions{})
	return res.Container, err
}

// addrString renders an unset address as "", the form the daemon's string API used.
func addrString(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}
