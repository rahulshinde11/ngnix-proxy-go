package webserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/network"
	"github.com/rahulshinde/nginx-proxy-go/internal/config"
	"github.com/rahulshinde/nginx-proxy-go/internal/dockerapi"
	"github.com/rahulshinde/nginx-proxy-go/internal/nginx"
)

type mockDockerClient struct {
	inspect map[string]types.ContainerJSON
}

var _ dockerapi.Client = (*mockDockerClient)(nil)

func (m *mockDockerClient) ContainerInspect(_ context.Context, id string) (types.ContainerJSON, error) {
	if c, ok := m.inspect[id]; ok {
		return c, nil
	}
	return types.ContainerJSON{}, errors.New("not found")
}

func (m *mockDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
	return nil, nil
}

func (m *mockDockerClient) Events(_ context.Context, _ types.EventsOptions) (<-chan events.Message, <-chan error) {
	eventCh := make(chan events.Message)
	errCh := make(chan error)
	close(eventCh)
	close(errCh)
	return eventCh, errCh
}

func (m *mockDockerClient) NetworkInspect(_ context.Context, id string, _ types.NetworkInspectOptions) (types.NetworkResource, error) {
	return types.NetworkResource{ID: id, Name: "frontend"}, nil
}

type fakeCommander struct {
	commands []string
}

func (f *fakeCommander) Command(name string, args ...string) nginx.Cmd {
	f.commands = append(f.commands, name)
	return &fakeCmd{}
}

type fakeCmd struct{}

func (*fakeCmd) CombinedOutput() ([]byte, error) { return nil, nil }

func (*fakeCmd) Start() error { return nil }

func TestHandleContainerStartEvent(t *testing.T) {
	oldHostname := os.Getenv("HOSTNAME")
	os.Setenv("HOSTNAME", "self-container")
	defer os.Setenv("HOSTNAME", oldHostname)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(originalWD, "../../"))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalWD)
	})

	client := &mockDockerClient{inspect: map[string]types.ContainerJSON{}}
	client.inspect["self-container"] = types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:   "self-container",
			Name: "/self",
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"frontend": {NetworkID: "net1"},
			},
		},
	}
	client.inspect["abc"] = types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:   "abc",
			Name: "/app",
		},
		Config: &container.Config{
			Env: []string{"VIRTUAL_HOST=https://example.com -> :8080/api"},
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"frontend": {NetworkID: "net1", IPAddress: "172.20.0.10"},
			},
		},
	}

	tmp := t.TempDir()
	cfg := config.NewConfig()
	cfg.ConfDir = filepath.Join(tmp, "nginx") + "/"
	cfg.ChallengeDir = filepath.Join(tmp, "acme") + "/"
	os.MkdirAll(filepath.Join(tmp, "acme"), 0o755)

	cmd := &fakeCommander{}
	ng := nginx.NewNginx(filepath.Join(tmp, "nginx", "conf.d", "default.conf"), cfg.ChallengeDir, cmd)

	server, err := NewWebServer(client, cfg, ng)
	if err != nil {
		t.Fatalf("NewWebServer error: %v", err)
	}

	event := events.Message{Type: "container", Action: "start", ID: "abc"}
	if err := server.HandleContainerEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleContainerEvent error: %v", err)
	}

	if len(server.hosts) == 0 {
		t.Fatalf("expected hosts to be populated")
	}
	if len(cmd.commands) == 0 {
		t.Fatalf("expected nginx update command to run")
	}
}
