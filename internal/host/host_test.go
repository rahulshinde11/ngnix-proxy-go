package host

import "testing"

func TestParseVirtualHost_BasicHTTP(t *testing.T) {
	config, err := ParseVirtualHost("example.com")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Hostname != "example.com" {
		t.Fatalf("expected hostname example.com, got %s", config.Hostname)
	}
	if config.ServerPort != 80 {
		t.Fatalf("expected server port 80, got %d", config.ServerPort)
	}
	if config.Scheme != "http" {
		t.Fatalf("expected scheme http, got %s", config.Scheme)
	}
	if config.Path != "/" {
		t.Fatalf("expected path /, got %s", config.Path)
	}
}

func TestParseVirtualHost_HTTPSWithPortAndPath(t *testing.T) {
	config, err := ParseVirtualHost("https://example.com:443/api -> :8080/internal")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Scheme != "https" {
		t.Fatalf("expected external scheme https, got %s", config.Scheme)
	}
	if config.ServerPort != 443 {
		t.Fatalf("expected server port 443, got %d", config.ServerPort)
	}
	if config.Path != "/internal" {
		t.Fatalf("expected container path /internal, got %s", config.Path)
	}
	if config.ContainerPort != 8080 {
		t.Fatalf("expected container port 8080, got %d", config.ContainerPort)
	}
	if config.ContainerScheme != "http" {
		t.Fatalf("expected container scheme http default, got %s", config.ContainerScheme)
	}
}

func TestParseVirtualHost_WithExtras(t *testing.T) {
	config, err := ParseVirtualHost("example.com -> :8000; proxy_read_timeout=90s; add_header X-Test value")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if len(config.Extras) != 2 {
		t.Fatalf("expected 2 extras, got %d", len(config.Extras))
	}
	if config.Extras[0] != "proxy_read_timeout=90s" {
		t.Fatalf("unexpected first extra: %s", config.Extras[0])
	}
	if config.Extras[1] != "add_header X-Test value" {
		t.Fatalf("unexpected second extra: %s", config.Extras[1])
	}
}

func TestParseVirtualHost_InvalidFormat(t *testing.T) {
	_, err := ParseVirtualHost("example.com -> -> :8080")
	if err == nil {
		t.Fatalf("expected error for invalid format")
	}
}

func TestHost_RemoveContainer_UpdatesUpstreams(t *testing.T) {
	// Create a host with multiple containers in upstream
	h := NewHost("example.com", 80)
	
	// Add containers
	container1 := &Container{
		ID:      "container1",
		Address: "172.17.0.2",
		Port:    8080,
		Scheme:  "http",
		Path:    "/",
	}
	
	container2 := &Container{
		ID:      "container2",
		Address: "172.17.0.3",
		Port:    8080,
		Scheme:  "http",
		Path:    "/",
	}
	
	container3 := &Container{
		ID:      "container3",
		Address: "172.17.0.4",
		Port:    8080,
		Scheme:  "http",
		Path:    "/",
	}
	
	// Add location with containers
	h.AddLocation("/", container1, map[string]string{})
	h.AddLocation("/", container2, map[string]string{})
	h.AddLocation("/", container3, map[string]string{})
	
	// Add upstream with all containers
	h.AddUpstream("example-80-0", []*Container{container1, container2, container3})
	
	// Verify initial state
	if len(h.Upstreams) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(h.Upstreams))
	}
	if len(h.Upstreams[0].Containers) != 3 {
		t.Fatalf("expected 3 containers in upstream, got %d", len(h.Upstreams[0].Containers))
	}
	
	// Remove container2
	removed := h.RemoveContainer("container2")
	if !removed {
		t.Fatal("expected RemoveContainer to return true")
	}
	
	// CRITICAL TEST: Verify upstream was updated
	if len(h.Upstreams) != 1 {
		t.Fatalf("expected 1 upstream after removal, got %d", len(h.Upstreams))
	}
	if len(h.Upstreams[0].Containers) != 2 {
		t.Fatalf("expected 2 containers in upstream after removal, got %d", len(h.Upstreams[0].Containers))
	}
	
	// Verify container2 is not in upstream
	for _, c := range h.Upstreams[0].Containers {
		if c.ID == "container2" {
			t.Fatal("container2 should have been removed from upstream")
		}
	}
	
	// Verify container1 and container3 are still there
	foundContainer1 := false
	foundContainer3 := false
	for _, c := range h.Upstreams[0].Containers {
		if c.ID == "container1" {
			foundContainer1 = true
		}
		if c.ID == "container3" {
			foundContainer3 = true
		}
	}
	if !foundContainer1 {
		t.Fatal("container1 should still be in upstream")
	}
	if !foundContainer3 {
		t.Fatal("container3 should still be in upstream")
	}
}

func TestHost_RemoveContainer_RemovesEmptyUpstream(t *testing.T) {
	// Create a host with a single container
	h := NewHost("example.com", 80)
	
	container := &Container{
		ID:      "container1",
		Address: "172.17.0.2",
		Port:    8080,
		Scheme:  "http",
		Path:    "/",
	}
	
	// Add location and upstream
	h.AddLocation("/", container, map[string]string{})
	h.AddUpstream("example-80-0", []*Container{container})
	
	// Verify initial state
	if len(h.Upstreams) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(h.Upstreams))
	}
	
	// Remove the only container
	removed := h.RemoveContainer("container1")
	if !removed {
		t.Fatal("expected RemoveContainer to return true")
	}
	
	// CRITICAL TEST: Verify upstream was completely removed
	if len(h.Upstreams) != 0 {
		t.Fatalf("expected 0 upstreams after removing last container, got %d", len(h.Upstreams))
	}
	
	// Verify host is empty
	if !h.IsEmpty() {
		t.Fatal("host should be empty after removing all containers")
	}
}

func TestHost_RemoveContainer_MultipleUpstreams(t *testing.T) {
	// Create a host with multiple upstreams
	h := NewHost("example.com", 80)
	
	container1 := &Container{ID: "c1", Address: "172.17.0.2", Port: 8080}
	container2 := &Container{ID: "c2", Address: "172.17.0.3", Port: 8080}
	container3 := &Container{ID: "c3", Address: "172.17.0.4", Port: 9090}
	
	// Add two locations with different containers
	h.AddLocation("/api", container1, map[string]string{})
	h.AddLocation("/api", container2, map[string]string{})
	h.AddLocation("/admin", container3, map[string]string{})
	
	// Add two separate upstreams
	h.AddUpstream("example-80-0", []*Container{container1, container2})
	h.AddUpstream("example-80-1", []*Container{container3})
	
	// Verify initial state
	if len(h.Upstreams) != 2 {
		t.Fatalf("expected 2 upstreams, got %d", len(h.Upstreams))
	}
	
	// Remove container1 (from first upstream)
	removed := h.RemoveContainer("c1")
	if !removed {
		t.Fatal("expected RemoveContainer to return true")
	}
	
	// Verify both upstreams still exist
	if len(h.Upstreams) != 2 {
		t.Fatalf("expected 2 upstreams after removal, got %d", len(h.Upstreams))
	}
	
	// Verify first upstream has one container
	if len(h.Upstreams[0].Containers) != 1 {
		t.Fatalf("expected 1 container in first upstream, got %d", len(h.Upstreams[0].Containers))
	}
	if h.Upstreams[0].Containers[0].ID != "c2" {
		t.Fatalf("expected c2 in first upstream, got %s", h.Upstreams[0].Containers[0].ID)
	}
	
	// Verify second upstream is unchanged
	if len(h.Upstreams[1].Containers) != 1 {
		t.Fatalf("expected 1 container in second upstream, got %d", len(h.Upstreams[1].Containers))
	}
	if h.Upstreams[1].Containers[0].ID != "c3" {
		t.Fatalf("expected c3 in second upstream, got %s", h.Upstreams[1].Containers[0].ID)
	}
}
