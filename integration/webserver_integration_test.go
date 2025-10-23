//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/rahulshinde/nginx-proxy-go/internal/processor"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestProcessVirtualHostsWithRealContainer(t *testing.T) {
	ctx := context.Background()

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	defer dockerClient.Close()

	networkName := fmt.Sprintf("proxy-integration-%d", time.Now().UnixNano())
	network, err := tc.GenericNetwork(ctx, tc.GenericNetworkRequest{
		NetworkRequest: tc.NetworkRequest{Name: networkName, CheckDuplicate: true},
	})
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
	defer network.Remove(ctx) //nolint:errcheck

	containerReq := tc.ContainerRequest{
		Image:        "nginx:1.27-alpine",
		Env:          map[string]string{"VIRTUAL_HOST": "integration.example.com"},
		Networks:     []string{networkName},
		ExposedPorts: []string{"80/tcp"},
		WaitingFor:   wait.ForHTTP("/").WithStartupTimeout(45 * time.Second),
	}

	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: containerReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
	defer container.Terminate(ctx) //nolint:errcheck

	inspect, err := dockerClient.ContainerInspect(ctx, container.GetContainerID())
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}

	knownNetworks := make(map[string]string)
	for name, net := range inspect.NetworkSettings.Networks {
		knownNetworks[net.NetworkID] = name
	}

	envMap := make(map[string]string)
	for _, e := range inspect.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	result := processor.ProcessVirtualHosts(inspect, envMap, knownNetworks)
	if len(result) == 0 {
		t.Fatalf("expected processed hosts, got none")
	}

	if _, exists := result["integration.example.com:80"]; !exists {
		t.Fatalf("expected host entry for integration.example.com:80, got keys: %v", keys(result))
	}
}

func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}


