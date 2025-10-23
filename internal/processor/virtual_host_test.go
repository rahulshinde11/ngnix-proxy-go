package processor

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

func TestProcessVirtualHosts(t *testing.T) {
	cont := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:   "123",
			Name: "/app",
		},
		Config: &container.Config{
			Env:          []string{"VIRTUAL_HOST=https://app.example.com -> :8080/api"},
			ExposedPorts: map[nat.Port]struct{}{nat.Port("80/tcp"): {}},
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"frontend": {NetworkID: "n1", IPAddress: "172.20.0.10"},
			},
		},
	}

	knownNetworks := map[string]string{"n1": "frontend"}
	env := map[string]string{"VIRTUAL_HOST": "https://app.example.com -> :8080/api"}

	result := ProcessVirtualHosts(cont, env, knownNetworks)
	if len(result) != 1 {
		t.Fatalf("expected 1 host, got %d", len(result))
	}

	h, ok := result["app.example.com:443"]
	if !ok {
		t.Fatalf("expected host key app.example.com:443")
	}
	if !h.SSLEnabled {
		t.Fatalf("expected SSL enabled")
	}

	loc, ok := h.Locations["/api"]
	if !ok {
		t.Fatalf("expected location /api")
	}
	containers := loc.GetContainers()
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].Port != 8080 {
		t.Fatalf("expected container port 8080, got %d", containers[0].Port)
	}
	if containers[0].Scheme != "http" {
		t.Fatalf("expected container scheme http, got %s", containers[0].Scheme)
	}
}
