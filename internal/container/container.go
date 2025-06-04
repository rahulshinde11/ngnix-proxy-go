package container

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
)

// Container represents a Docker container with its configuration
type Container struct {
	ID          string
	Name        string
	Environment map[string]string
	Networks    []string
	IPAddress   string
	Port        int
	Scheme      string
}

// NewContainer creates a new Container instance from a Docker container
func NewContainer(container types.ContainerJSON) *Container {
	return &Container{
		ID:          container.ID,
		Name:        strings.TrimPrefix(container.Name, "/"),
		Environment: parseEnvironment(container.Config.Labels),
		Networks:    getNetworks(container.NetworkSettings),
		IPAddress:   getIPAddress(container.NetworkSettings),
		Port:        getPort(container),
		Scheme:      getScheme(container.Config.Labels),
	}
}

// GetEnvMap returns a map of environment variables from container labels
func GetEnvMap(container types.ContainerJSON) map[string]string {
	return parseEnvironment(container.Config.Labels)
}

// parseEnvironment converts container labels to environment variables
func parseEnvironment(labels map[string]string) map[string]string {
	env := make(map[string]string)
	for k, v := range labels {
		if strings.HasPrefix(k, "VIRTUAL_HOST") || strings.HasPrefix(k, "STATIC_VIRTUAL_HOST") {
			env[k] = v
		}
	}
	return env
}

// getNetworks returns a list of network names the container is connected to
func getNetworks(networkSettings *types.NetworkSettings) []string {
	networks := make([]string, 0, len(networkSettings.Networks))
	for network := range networkSettings.Networks {
		networks = append(networks, network)
	}
	return networks
}

// getIPAddress returns the IP address of the container
func getIPAddress(networkSettings *types.NetworkSettings) string {
	for _, network := range networkSettings.Networks {
		if network.IPAddress != "" {
			return network.IPAddress
		}
	}
	return ""
}

// getPort returns the port of the container
func getPort(container types.ContainerJSON) int {
	// Check for VIRTUAL_PORT
	if port, ok := container.Config.Labels["VIRTUAL_PORT"]; ok {
		var p int
		if _, err := fmt.Sscanf(port, "%d", &p); err == nil {
			return p
		}
	}

	// Check exposed ports
	if len(container.Config.ExposedPorts) == 1 {
		for port := range container.Config.ExposedPorts {
			portStr := port.Port()
			var p int
			if _, err := fmt.Sscanf(portStr, "%d", &p); err == nil {
				return p
			}
		}
	}

	// Default to 80
	return 80
}

// getScheme returns the scheme of the container
func getScheme(labels map[string]string) string {
	if _, ok := labels["LETSENCRYPT_HOST"]; ok {
		return "https"
	}
	return "http"
}

// IsReachable checks if the container is reachable through known networks
func (c *Container) IsReachable(knownNetworks map[string]string) bool {
	for _, network := range c.Networks {
		if _, ok := knownNetworks[network]; ok {
			return true
		}
	}
	return false
}
