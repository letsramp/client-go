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
	"strconv"
	"time"

	dockerTypes "github.com/docker/docker/api/types"
	dockerContainer "github.com/docker/docker/api/types/container"
	dockerImage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/letsramp/client-go/types"
	log "github.com/sirupsen/logrus"
)

var logger = log.WithField("module", "client")

type DockerLCM struct {
	// docker client
	client *docker.Client
	// docker container object
	Container *dockerTypes.ContainerJSON
	// docker network ID that the work is in
	NetworkID string
	// docker network name
	NetworkName string
}

var (
	DockerEngineAPIVersion = "1.44"
)

// generic docker LCM to communicate with docker API
func NewDockerLCM() (*DockerLCM, error) {
	client, err := docker.NewClientWithOpts(docker.WithVersion(DockerEngineAPIVersion))
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
func (d *DockerLCM) RestartWithNewPorts(container *dockerTypes.ContainerJSON, ports []int) error {
	ctx := context.Background()

	// get relevant fields from original container
	config := container.Config
	networkSettings := container.NetworkSettings
	hostConfig := container.HostConfig
	name := container.Name

	// add new ports
	if config.ExposedPorts == nil {
		config.ExposedPorts = nat.PortSet{}
	}

	for _, p := range ports {
		newPort, err := nat.NewPort("tcp", strconv.Itoa(p))
		if err != nil {
			return err
		}
		config.ExposedPorts[newPort] = struct{}{}
		hostConfig.PortBindings[newPort] = []nat.PortBinding{
			{
				HostIP:   "127.0.0.1",
				HostPort: strconv.Itoa(p),
			},
		}
	}

	// stop and remove existing container
	logger.Infof("attempting to kill container %s", name)
	if err := d.client.ContainerRemove(ctx, container.ID, dockerContainer.RemoveOptions{Force: true}); err != nil {
		logger.Errorf("failed to remove container %s: %v", name, err)
		return fmt.Errorf("failed to remove container %s: %v", name, err)
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: networkSettings.Networks,
	}

	// create and run it in background
	resp, err := d.client.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, name)
	if err != nil {
		logger.Errorf("failed to create a container %s: %v", name, err)
		return fmt.Errorf("failed to create a container %s: %w", name, err)
	}

	if err := d.client.ContainerStart(ctx, resp.ID, dockerContainer.StartOptions{}); err != nil {
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
	vol, err := dockerLcm.client.VolumeCreate(context.Background(), volume.CreateOptions{
		Name: containerName,
	})

	if err != nil {
		return fmt.Errorf("error occurred while creating volume: %v", err)
	}
	imagePath := fmt.Sprintf("%s:%s", image, tag)
	_, _, err = dockerLcm.client.ImageInspectWithRaw(context.Background(), imagePath)
	if err != nil {
		out, err := dockerLcm.client.ImagePull(context.Background(), imagePath, dockerImage.PullOptions{})
		if err != nil {
			return fmt.Errorf("error occurred while pulling image %s: %v", imagePath, err)
		}
		reader := bufio.NewReader(out)
		if _, err = io.ReadAll(reader); err != nil {
			return fmt.Errorf("error reading image data at pull: %v", err)
		}
		out.Close()
	}
	workerContainer, err := dockerLcm.client.ContainerCreate(
		context.Background(),
		&dockerContainer.Config{
			Image: imagePath,
			Volumes: map[string]struct{}{
				types.DefaultWorkerConfigDirPath: {},
			},
			ExposedPorts: map[nat.Port]struct{}{
				nat.Port(containerExposedPort): {},
			},
		},
		&dockerContainer.HostConfig{
			Binds: []string{
				fmt.Sprintf("%s:%s:rw", vol.Name, types.DefaultWorkerConfigDirPath),
			},
			RestartPolicy: dockerContainer.RestartPolicy{
				Name: "always",
			},
			PortBindings: map[nat.Port][]nat.PortBinding{
				nat.Port(containerExposedPort): {{HostPort: fmt.Sprintf("%d", hostPort)}},
			},
		},
		&network.NetworkingConfig{},
		nil,
		containerName)
	if err != nil {
		return fmt.Errorf("error occurred while creating container %s: %v", containerName, err)
	}

	err = dockerLcm.client.ContainerStart(context.Background(), workerContainer.ID, dockerContainer.StartOptions{})
	if err != nil {
		return fmt.Errorf("error occurred while starting container %s: %v", containerName, err)
	}

	containerInfo, err := dockerLcm.client.ContainerInspect(context.Background(), workerContainer.ID)
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

		err = dockerLcm.client.ContainerStop(context.Background(), dockerLcm.Container.ID, dockerContainer.StopOptions{})
		if err != nil {
			return fmt.Errorf("error occurred while stoping container %s: %v", containerName, err)
		}
		err = dockerLcm.client.NetworkDisconnect(context.Background(), workerNetwork.NetworkID, dockerLcm.Container.ID, true)
		if err != nil {
			return fmt.Errorf("error occurred while disconnecting network for %s container: %v", containerName, err)
		}
		err = dockerLcm.client.NetworkConnect(context.Background(), targetNetwork.ID, dockerLcm.Container.ID, &network.EndpointSettings{})
		if err != nil {
			return fmt.Errorf("error occurred while attaching container %s to %s: %v", containerName, targetNetwork.ID, err)
		}
		err = dockerLcm.client.ContainerStart(context.Background(), dockerLcm.Container.ID, dockerContainer.StartOptions{})
		if err != nil {
			return fmt.Errorf("error occurred while starting container %s: %v", containerName, err)
		}
	}

	// wait for container to be ready
	err = waitForContainerStarted(dockerLcm.client, context.Background(), workerContainer.ID)
	if err != nil {
		return fmt.Errorf("error occurred while waiting for container %s to start: %v", containerName, err)
	}
	return nil
}

func waitForContainerStarted(client *docker.Client, ctx context.Context, containerID string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil // Context done, no error
		case <-ticker.C:
			// Check container status
			status, err := client.ContainerInspect(ctx, containerID)
			if err != nil {
				return fmt.Errorf("failed to inspect container %s: %w", containerID, err)
			}

			if status.State.Running {
				if isContainerReady(status) {
					return nil
				}
			}
		}
	}
}

func isContainerReady(status dockerTypes.ContainerJSON) bool {
	return status.State.Health != nil && status.State.Health.Status == "healthy"
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
	_ = dockerLcm.client.ContainerRemove(context.Background(), workerContainer.ID, dockerContainer.RemoveOptions{Force: true})

	// delete docker volume
	err = dockerLcm.client.VolumeRemove(context.Background(), containerName, true)
	if err != nil {
		return fmt.Errorf("error occurred while removing volume %s: %v", containerName, err)
	}

	return nil
}

func findContainerByName(containerName string) (*dockerTypes.Container, error) {
	dockerLcm, err := NewDockerLCM()
	if err != nil {
		return nil, err
	}

	containers, err := dockerLcm.client.ContainerList(context.Background(), dockerContainer.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	for _, c := range containers {
		if c.Names[0] == "/"+containerName {
			return &c, nil
		}
	}

	return nil, fmt.Errorf("container %s not found", containerName)
}

func (d *DockerLCM) FindNetworkByName(networkName string) (*dockerTypes.NetworkResource, error) {
	networks, err := d.client.NetworkList(context.Background(), dockerTypes.NetworkListOptions{})
	if err != nil {
		return nil, err
	}

	var networkID string
	for _, n := range networks {
		if n.Name == networkName {
			networkID = n.ID
			break
		}
	}

	if networkID == "" {
		return nil, fmt.Errorf("failed to find network %s", networkID)
	}

	network, err := d.client.NetworkInspect(context.Background(), networkID, dockerTypes.NetworkInspectOptions{})
	if err != nil {
		return nil, err
	}

	return &network, nil
}

// find container in docker using address and IP
// there could be some edges cases that this does not properly handle
func (d *DockerLCM) FindContainerByBoundAddress(address string) (*dockerTypes.ContainerJSON, error) {
	containers, err := d.client.ContainerList(context.Background(), dockerContainer.ListOptions{})
	if err != nil {
		return nil, err
	}

	addrPort, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", address, err)
	}

	for _, c := range containers {
		container, err := d.client.ContainerInspect(context.Background(), c.ID)
		if err != nil {
			return nil, err
		}

		portBindings := container.HostConfig.PortBindings
		for _, bindings := range portBindings {
			for _, b := range bindings {
				if b.HostPort == strconv.Itoa(addrPort.Port) && isIPSame(b.HostIP, addrPort.IP.String()) {
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

func IsInDefaultBridgeNetwork(container *dockerTypes.ContainerJSON) bool {
	networkName, _ := getNetworkEndpointOfContainer(container)
	return networkName == "bridge"
}

// this assumes container belongs to a single network only
func getNetworkEndpointOfContainer(container *dockerTypes.ContainerJSON) (string, *network.EndpointSettings) {
	if container.NetworkSettings == nil {
		return "", nil
	}

	for name, e := range container.NetworkSettings.Networks {
		return name, e
	}

	return "", nil
}
