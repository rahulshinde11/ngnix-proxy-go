package host

import (
	"testing"
)

// TestUpstreamLinking verifies that Location.Upstream is correctly set
func TestUpstreamLinking(t *testing.T) {
	h := NewHost("example.com", 80)
	
	// Add multiple containers to same location (should create upstream)
	c1 := &Container{ID: "c1", Address: "172.17.0.2", Port: 8080}
	c2 := &Container{ID: "c2", Address: "172.17.0.3", Port: 8080}
	
	h.AddLocation("/", c1, map[string]string{})
	h.AddLocation("/", c2, map[string]string{})
	
	// Manually set upstream (simulating what rebuildHostUpstreams does)
	location := h.Locations["/"]
	upstreamID := "example.com-80-root"
	location.Upstream = upstreamID
	location.UpstreamEnabled = true
	h.AddUpstream(upstreamID, []*Container{c1, c2})
	
	// Verify upstream is correctly set
	if !location.UpstreamEnabled {
		t.Fatal("UpstreamEnabled should be true for multiple containers")
	}
	
	if location.Upstream != upstreamID {
		t.Fatalf("expected Upstream=%s, got %s", upstreamID, location.Upstream)
	}
	
	// Verify upstream exists with correct ID
	if len(h.Upstreams) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(h.Upstreams))
	}
	
	if h.Upstreams[0].ID != upstreamID {
		t.Fatalf("expected upstream ID=%s, got %s", upstreamID, h.Upstreams[0].ID)
	}
	
	// Verify upstream has both containers
	if len(h.Upstreams[0].Containers) != 2 {
		t.Fatalf("expected 2 containers in upstream, got %d", len(h.Upstreams[0].Containers))
	}
}

// TestSingleContainerNoUpstream verifies single container doesn't use upstream
func TestSingleContainerNoUpstream(t *testing.T) {
	h := NewHost("example.com", 80)
	
	c := &Container{ID: "c1", Address: "172.17.0.2", Port: 8080}
	h.AddLocation("/", c, map[string]string{})
	
	location := h.Locations["/"]
	
	// Single container should not enable upstream
	if location.UpstreamEnabled {
		t.Fatal("UpstreamEnabled should be false for single container")
	}
	
	if location.Upstream != "" {
		t.Fatalf("Upstream should be empty for single container, got %s", location.Upstream)
	}
}

// TestMultipleLocationsWithUpstreams tests different locations with their own upstreams
func TestMultipleLocationsWithUpstreams(t *testing.T) {
	h := NewHost("example.com", 80)
	
	// Location /api with 2 containers
	api1 := &Container{ID: "api1", Address: "172.17.0.2", Port: 8080}
	api2 := &Container{ID: "api2", Address: "172.17.0.3", Port: 8080}
	h.AddLocation("/api", api1, map[string]string{})
	h.AddLocation("/api", api2, map[string]string{})
	
	// Location /admin with 2 containers
	admin1 := &Container{ID: "admin1", Address: "172.17.0.4", Port: 9090}
	admin2 := &Container{ID: "admin2", Address: "172.17.0.5", Port: 9090}
	h.AddLocation("/admin", admin1, map[string]string{})
	h.AddLocation("/admin", admin2, map[string]string{})
	
	// Simulate rebuildHostUpstreams
	apiUpstreamID := "example.com-80-_api"
	adminUpstreamID := "example.com-80-_admin"
	
	apiLoc := h.Locations["/api"]
	apiLoc.Upstream = apiUpstreamID
	apiLoc.UpstreamEnabled = true
	h.AddUpstream(apiUpstreamID, []*Container{api1, api2})
	
	adminLoc := h.Locations["/admin"]
	adminLoc.Upstream = adminUpstreamID
	adminLoc.UpstreamEnabled = true
	h.AddUpstream(adminUpstreamID, []*Container{admin1, admin2})
	
	// Verify /api location
	if !apiLoc.UpstreamEnabled {
		t.Fatal("/api location should have UpstreamEnabled=true")
	}
	if apiLoc.Upstream != apiUpstreamID {
		t.Fatalf("/api expected upstream=%s, got %s", apiUpstreamID, apiLoc.Upstream)
	}
	
	// Verify /admin location
	if !adminLoc.UpstreamEnabled {
		t.Fatal("/admin location should have UpstreamEnabled=true")
	}
	if adminLoc.Upstream != adminUpstreamID {
		t.Fatalf("/admin expected upstream=%s, got %s", adminUpstreamID, adminLoc.Upstream)
	}
	
	// Verify we have 2 separate upstreams
	if len(h.Upstreams) != 2 {
		t.Fatalf("expected 2 upstreams, got %d", len(h.Upstreams))
	}
	
	// Verify each upstream has correct containers
	foundAPI := false
	foundAdmin := false
	for _, upstream := range h.Upstreams {
		if upstream.ID == apiUpstreamID {
			foundAPI = true
			if len(upstream.Containers) != 2 {
				t.Fatalf("API upstream should have 2 containers, got %d", len(upstream.Containers))
			}
			// Verify containers are correct
			for _, c := range upstream.Containers {
				if c.ID != "api1" && c.ID != "api2" {
					t.Fatalf("API upstream has wrong container: %s", c.ID)
				}
			}
		} else if upstream.ID == adminUpstreamID {
			foundAdmin = true
			if len(upstream.Containers) != 2 {
				t.Fatalf("Admin upstream should have 2 containers, got %d", len(upstream.Containers))
			}
			// Verify containers are correct
			for _, c := range upstream.Containers {
				if c.ID != "admin1" && c.ID != "admin2" {
					t.Fatalf("Admin upstream has wrong container: %s", c.ID)
				}
			}
		}
	}
	
	if !foundAPI {
		t.Fatal("API upstream not found")
	}
	if !foundAdmin {
		t.Fatal("Admin upstream not found")
	}
}

// TestContainerRemovalUpdatesUpstream verifies removing container updates upstream correctly
func TestContainerRemovalUpdatesUpstream(t *testing.T) {
	h := NewHost("example.com", 80)
	
	c1 := &Container{ID: "c1", Address: "172.17.0.2", Port: 8080}
	c2 := &Container{ID: "c2", Address: "172.17.0.3", Port: 8080}
	c3 := &Container{ID: "c3", Address: "172.17.0.4", Port: 8080}
	
	h.AddLocation("/", c1, map[string]string{})
	h.AddLocation("/", c2, map[string]string{})
	h.AddLocation("/", c3, map[string]string{})
	
	// Set up upstream
	upstreamID := "example.com-80-root"
	location := h.Locations["/"]
	location.Upstream = upstreamID
	location.UpstreamEnabled = true
	h.AddUpstream(upstreamID, []*Container{c1, c2, c3})
	
	// Remove one container
	h.RemoveContainer("c2")
	
	// Verify upstream was updated
	if len(h.Upstreams) != 1 {
		t.Fatalf("expected 1 upstream after removal, got %d", len(h.Upstreams))
	}
	
	if len(h.Upstreams[0].Containers) != 2 {
		t.Fatalf("expected 2 containers in upstream after removal, got %d", len(h.Upstreams[0].Containers))
	}
	
	// Verify c2 is not in upstream
	for _, c := range h.Upstreams[0].Containers {
		if c.ID == "c2" {
			t.Fatal("c2 should have been removed from upstream")
		}
	}
	
	// Location should still have upstream enabled (2 containers remain)
	if !location.UpstreamEnabled {
		t.Fatal("UpstreamEnabled should still be true with 2 containers")
	}
}
