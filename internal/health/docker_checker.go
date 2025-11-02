package health

import (
	"context"
	"time"

	"github.com/rahulshinde/nginx-proxy-go/internal/dockerapi"
)

// DockerChecker checks if Docker daemon is accessible
type DockerChecker struct {
	client dockerapi.Client
}

// NewDockerChecker creates a new Docker health checker
func NewDockerChecker(client dockerapi.Client) *DockerChecker {
	return &DockerChecker{
		client: client,
	}
}

// Check performs the Docker health check
func (c *DockerChecker) Check() Check {
	start := time.Now()
	
	// Try to ping Docker daemon by listing containers
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := c.client.ContainerList(ctx, dockerapi.ListOptions{})
	
	latency := time.Since(start)
	
	if err != nil {
		return Check{
			Name:    "docker",
			Status:  StatusUnhealthy,
			Message: "cannot connect to Docker daemon: " + err.Error(),
			Latency: latency,
		}
	}
	
	return Check{
		Name:    "docker",
		Status:  StatusHealthy,
		Message: "Docker daemon is accessible",
		Latency: latency,
	}
}
