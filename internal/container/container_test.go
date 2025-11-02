package container

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

func TestNewContainer(t *testing.T) {
	containerJSON := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:   "abc123",
			Name: "/test-container",
		},
		Config: &container.Config{
			Labels: map[string]string{
				"VIRTUAL_HOST":    "example.com",
				"LETSENCRYPT_HOST": "example.com",
			},
			ExposedPorts: nat.PortSet{
				"8080/tcp": struct{}{},
			},
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"frontend": {
					IPAddress: "172.17.0.2",
				},
			},
		},
	}

	c := NewContainer(containerJSON)

	if c.ID != "abc123" {
		t.Errorf("expected ID 'abc123', got '%s'", c.ID)
	}

	if c.Name != "test-container" {
		t.Errorf("expected Name 'test-container', got '%s'", c.Name)
	}

	if c.IPAddress != "172.17.0.2" {
		t.Errorf("expected IPAddress '172.17.0.2', got '%s'", c.IPAddress)
	}

	if c.Port != 8080 {
		t.Errorf("expected Port 8080, got %d", c.Port)
	}

	if c.Scheme != "https" {
		t.Errorf("expected Scheme 'https', got '%s'", c.Scheme)
	}

	if _, ok := c.Environment["VIRTUAL_HOST"]; !ok {
		t.Error("expected VIRTUAL_HOST in environment")
	}

	if len(c.Networks) != 1 || c.Networks[0] != "frontend" {
		t.Errorf("expected Networks ['frontend'], got %v", c.Networks)
	}
}

func TestGetEnvMap(t *testing.T) {
	containerJSON := types.ContainerJSON{
		Config: &container.Config{
			Labels: map[string]string{
				"VIRTUAL_HOST":        "example.com",
				"PROXY_BASIC_AUTH":    "user:pass",
				"STATIC_VIRTUAL_HOST": "static.example.com",
				"OTHER_LABEL":         "ignored",
			},
		},
	}

	env := GetEnvMap(containerJSON)

	if len(env) != 3 {
		t.Errorf("expected 3 environment variables, got %d", len(env))
	}

	if env["VIRTUAL_HOST"] != "example.com" {
		t.Errorf("expected VIRTUAL_HOST='example.com', got '%s'", env["VIRTUAL_HOST"])
	}

	if env["PROXY_BASIC_AUTH"] != "user:pass" {
		t.Errorf("expected PROXY_BASIC_AUTH='user:pass', got '%s'", env["PROXY_BASIC_AUTH"])
	}

	if env["STATIC_VIRTUAL_HOST"] != "static.example.com" {
		t.Errorf("expected STATIC_VIRTUAL_HOST='static.example.com', got '%s'", env["STATIC_VIRTUAL_HOST"])
	}

	if _, ok := env["OTHER_LABEL"]; ok {
		t.Error("expected OTHER_LABEL to be ignored")
	}
}

func TestGetPort(t *testing.T) {
	tests := []struct {
		name         string
		containerJSON types.ContainerJSON
		expectedPort int
	}{
		{
			name: "port from VIRTUAL_PORT label",
			containerJSON: types.ContainerJSON{
				Config: &container.Config{
					Labels: map[string]string{
						"VIRTUAL_PORT": "3000",
					},
				},
			},
			expectedPort: 3000,
		},
		{
			name: "port from single exposed port",
			containerJSON: types.ContainerJSON{
				Config: &container.Config{
					Labels: map[string]string{},
					ExposedPorts: nat.PortSet{
						"8080/tcp": struct{}{},
					},
				},
			},
			expectedPort: 8080,
		},
		{
			name: "default port when no exposed ports",
			containerJSON: types.ContainerJSON{
				Config: &container.Config{
					Labels:       map[string]string{},
					ExposedPorts: nat.PortSet{},
				},
			},
			expectedPort: 80,
		},
		{
			name: "default port when multiple exposed ports",
			containerJSON: types.ContainerJSON{
				Config: &container.Config{
					Labels: map[string]string{},
					ExposedPorts: nat.PortSet{
						"8080/tcp": struct{}{},
						"9090/tcp": struct{}{},
					},
				},
			},
			expectedPort: 80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := getPort(tt.containerJSON)
			if port != tt.expectedPort {
				t.Errorf("expected port %d, got %d", tt.expectedPort, port)
			}
		})
	}
}

func TestGetScheme(t *testing.T) {
	tests := []struct {
		name           string
		labels         map[string]string
		expectedScheme string
	}{
		{
			name: "https when LETSENCRYPT_HOST is present",
			labels: map[string]string{
				"LETSENCRYPT_HOST": "example.com",
			},
			expectedScheme: "https",
		},
		{
			name:           "http when LETSENCRYPT_HOST is not present",
			labels:         map[string]string{},
			expectedScheme: "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := getScheme(tt.labels)
			if scheme != tt.expectedScheme {
				t.Errorf("expected scheme '%s', got '%s'", tt.expectedScheme, scheme)
			}
		})
	}
}

func TestGetIPAddress(t *testing.T) {
	tests := []struct {
		name        string
		settings    *types.NetworkSettings
		expectedIP  string
	}{
		{
			name: "single network with IP",
			settings: &types.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"frontend": {
						IPAddress: "172.17.0.2",
					},
				},
			},
			expectedIP: "172.17.0.2",
		},
		{
			name: "multiple networks, first with IP",
			settings: &types.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"frontend": {
						IPAddress: "172.17.0.2",
					},
					"backend": {
						IPAddress: "172.18.0.3",
					},
				},
			},
			expectedIP: "172.17.0.2", // Returns first found
		},
		{
			name: "no IP address",
			settings: &types.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"frontend": {
						IPAddress: "",
					},
				},
			},
			expectedIP: "",
		},
		{
			name: "empty networks",
			settings: &types.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{},
			},
			expectedIP: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := getIPAddress(tt.settings)
			if tt.expectedIP != "" && ip == "" {
				t.Errorf("expected IP '%s', got empty string", tt.expectedIP)
			} else if tt.expectedIP == "" && ip != "" {
				t.Errorf("expected empty IP, got '%s'", ip)
			}
		})
	}
}

func TestGetNetworks(t *testing.T) {
	settings := &types.NetworkSettings{
		Networks: map[string]*network.EndpointSettings{
			"frontend": {
				IPAddress: "172.17.0.2",
			},
			"backend": {
				IPAddress: "172.18.0.3",
			},
		},
	}

	networks := getNetworks(settings)

	if len(networks) != 2 {
		t.Errorf("expected 2 networks, got %d", len(networks))
	}

	networkMap := make(map[string]bool)
	for _, net := range networks {
		networkMap[net] = true
	}

	if !networkMap["frontend"] {
		t.Error("expected 'frontend' network")
	}

	if !networkMap["backend"] {
		t.Error("expected 'backend' network")
	}
}

func TestContainer_IsReachable(t *testing.T) {
	c := &Container{
		Networks: []string{"frontend", "backend"},
	}

	tests := []struct {
		name           string
		knownNetworks  map[string]string
		expectedResult bool
	}{
		{
			name: "reachable through frontend",
			knownNetworks: map[string]string{
				"frontend": "net-123",
			},
			expectedResult: true,
		},
		{
			name: "reachable through backend",
			knownNetworks: map[string]string{
				"backend": "net-456",
			},
			expectedResult: true,
		},
		{
			name: "not reachable through unknown network",
			knownNetworks: map[string]string{
				"other": "net-789",
			},
			expectedResult: false,
		},
		{
			name:           "not reachable with empty known networks",
			knownNetworks:  map[string]string{},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.IsReachable(tt.knownNetworks)
			if result != tt.expectedResult {
				t.Errorf("expected IsReachable=%v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestParseEnvironment(t *testing.T) {
	labels := map[string]string{
		"VIRTUAL_HOST":        "example.com",
		"STATIC_VIRTUAL_HOST": "static.example.com",
		"PROXY_BASIC_AUTH":    "user:pass",
		"com.docker.compose":  "ignored",
		"version":             "1.0",
	}

	env := parseEnvironment(labels)

	expectedKeys := []string{"VIRTUAL_HOST", "STATIC_VIRTUAL_HOST", "PROXY_BASIC_AUTH"}
	unexpectedKeys := []string{"com.docker.compose", "version"}

	for _, key := range expectedKeys {
		if _, ok := env[key]; !ok {
			t.Errorf("expected key '%s' in environment", key)
		}
	}

	for _, key := range unexpectedKeys {
		if _, ok := env[key]; ok {
			t.Errorf("unexpected key '%s' in environment", key)
		}
	}
}
