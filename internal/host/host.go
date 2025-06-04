package host

import (
	"fmt"
	"strings"
)

// Host represents a virtual host configuration
type Host struct {
	Hostname         string
	Port             int
	SSLEnabled       bool
	SSLFile          string
	IsRedirect       bool
	RedirectHostname string
	IsDown           bool
	IsDefaultServer  bool
	BasicAuth        bool
	BasicAuthFile    string
	SSLRedirect      bool
	Locations        map[string]*Location
	Upstreams        []*Upstream
	Scheme           string
	IsStatic         bool
	Extras           *ExtrasMap
}

// Upstream represents a group of backend servers
type Upstream struct {
	ID         string
	Containers []*Container
}

// Container represents a container that serves a location
type Container struct {
	ID      string
	Address string
	Port    int
	Scheme  string
	Path    string
}

// Location represents a location block in nginx configuration
type Location struct {
	Path             string
	Upstream         string
	Scheme           string
	ContainerAddress string
	ContainerPort    int
	ContainerPath    string
	WebSocket        bool
	HTTP             bool
	BasicAuth        bool
	BasicAuthFile    string
	InjectedConfigs  []string
	Extras           *ExtrasMap
	Containers       map[string]*Container // Map of container ID to Container
	UpstreamEnabled  bool                  // Whether this location uses upstream
}

// NewHost creates a new Host instance
func NewHost(hostname string, port int) *Host {
	return &Host{
		Hostname:  hostname,
		Port:      port,
		Locations: make(map[string]*Location),
		Upstreams: make([]*Upstream, 0),
		Scheme:    "http",
		Extras:    NewExtrasMap(),
	}
}

// UpdateExtras updates the host's extras with new values
func (h *Host) UpdateExtras(extras map[string]interface{}) {
	extrasMap := make(map[string]string)
	for k, v := range extras {
		switch val := v.(type) {
		case string:
			extrasMap[k] = val
		case bool:
			extrasMap[k] = fmt.Sprintf("%v", val)
		case int:
			extrasMap[k] = fmt.Sprintf("%d", val)
		default:
			extrasMap[k] = fmt.Sprintf("%v", val)
		}
	}
	h.Extras.Update(extrasMap)
}

// UpdateExtrasContent updates a single extra value
func (h *Host) UpdateExtrasContent(key string, value interface{}) {
	h.Extras.Set(key, value)
}

// AddLocation adds a location to the host
func (h *Host) AddLocation(path string, container *Container, extras map[string]string) {
	if path == "" {
		path = "/"
	}

	// Get or create location
	location, exists := h.Locations[path]
	if !exists {
		location = &Location{
			Path:            path,
			Containers:      make(map[string]*Container),
			Extras:          NewExtrasMap(),
			InjectedConfigs: make([]string, 0),
		}
		h.Locations[path] = location
	}

	// Add container
	location.Containers[container.ID] = container

	// Update extras
	if len(extras) > 0 {
		location.Extras.Update(extras)
	}

	// Enable upstream if multiple containers
	if len(location.Containers) > 1 {
		location.UpstreamEnabled = true
	}
}

// UpdateExtras updates the location's extras with new values
func (l *Location) UpdateExtras(extras map[string]interface{}) {
	extrasMap := make(map[string]string)
	for k, v := range extras {
		switch val := v.(type) {
		case string:
			extrasMap[k] = val
		case bool:
			extrasMap[k] = fmt.Sprintf("%v", val)
		case int:
			extrasMap[k] = fmt.Sprintf("%d", val)
		default:
			extrasMap[k] = fmt.Sprintf("%v", val)
		}
	}
	l.Extras.Update(extrasMap)
}

// UpdateExtrasContent updates a single extra value in the location
func (l *Location) UpdateExtrasContent(key string, value interface{}) {
	l.Extras.Set(key, value)
}

// AddUpstream adds an upstream to the host
func (h *Host) AddUpstream(id string, containers []*Container) {
	h.Upstreams = append(h.Upstreams, &Upstream{
		ID:         id,
		Containers: containers,
	})
}

// SetSSL enables SSL for the host
func (h *Host) SetSSL(enabled bool, sslFile string) {
	h.SSLEnabled = enabled
	h.SSLFile = sslFile
	if enabled {
		h.Scheme = "https"
	}
}

// SetRedirect sets up a redirect for the host
func (h *Host) SetRedirect(redirectHostname string) {
	h.IsRedirect = true
	h.RedirectHostname = redirectHostname
}

// SetBasicAuth enables basic authentication for the host
func (h *Host) SetBasicAuth(enabled bool, authFile string) {
	h.BasicAuth = enabled
	h.BasicAuthFile = authFile
}

// SetLocationBasicAuth enables basic authentication for a specific location
func (h *Host) SetLocationBasicAuth(path string, enabled bool, authFile string) {
	if loc, ok := h.Locations[path]; ok {
		loc.BasicAuth = enabled
		loc.BasicAuthFile = authFile
	}
}

// AddInjectedConfig adds an injected configuration line to a location
func (h *Host) AddInjectedConfig(path, config string) {
	if loc, ok := h.Locations[path]; ok {
		loc.InjectedConfigs = append(loc.InjectedConfigs, config)
	}
}

// ParseVirtualHost parses a VIRTUAL_HOST environment variable and extracts extras
func ParseVirtualHost(virtualHost string) (hostname string, port int, path string, extras []string, err error) {
	// Split extras (semicolon separated)
	parts := strings.SplitN(virtualHost, ";", 2)
	mainPart := parts[0]
	extraLines := []string{}
	if len(parts) > 1 {
		for _, extra := range strings.Split(parts[1], ";") {
			extra = strings.TrimSpace(extra)
			if extra != "" {
				extraLines = append(extraLines, extra)
			}
		}
	}

	// Now parse the main part as before
	mainParts := strings.Split(mainPart, "->")
	if len(mainParts) > 2 {
		return "", 0, "", nil, fmt.Errorf("invalid VIRTUAL_HOST format: %s", virtualHost)
	}

	// Parse the hostname part
	hostPart := strings.TrimSpace(mainParts[0])
	if strings.HasPrefix(hostPart, "https://") {
		hostPart = strings.TrimPrefix(hostPart, "https://")
	} else if strings.HasPrefix(hostPart, "http://") {
		hostPart = strings.TrimPrefix(hostPart, "http://")
	} else if strings.HasPrefix(hostPart, "wss://") {
		hostPart = strings.TrimPrefix(hostPart, "wss://")
	} else if strings.HasPrefix(hostPart, "ws://") {
		hostPart = strings.TrimPrefix(hostPart, "ws://")
	}

	// Split hostname and port
	hostPort := strings.Split(hostPart, ":")
	hostname = hostPort[0]
	port = 80
	if len(hostPort) > 1 {
		_, err := fmt.Sscanf(hostPort[1], "%d", &port)
		if err != nil {
			return "", 0, "", nil, fmt.Errorf("invalid port in VIRTUAL_HOST: %s", virtualHost)
		}
	}

	// Parse the path part if present
	path = "/"
	if len(mainParts) > 1 {
		pathPart := strings.TrimSpace(mainParts[1])
		if pathPart != "" {
			path = pathPart
		}
	}

	return hostname, port, path, extraLines, nil
}

// MergeExtras merges extras/injected configs from another host or location
func (h *Host) MergeExtras(extras []string) {
	extrasMap := make(map[string]string)
	for i, extra := range extras {
		extrasMap[fmt.Sprintf("extra_%d", i)] = extra
	}
	h.Extras.Update(extrasMap)
}

func (l *Location) MergeExtras(extras []string) {
	extrasMap := make(map[string]string)
	for i, extra := range extras {
		extrasMap[fmt.Sprintf("extra_%d", i)] = extra
	}
	l.Extras.Update(extrasMap)
}

// NewLocation creates a new Location instance
func NewLocation(path string) *Location {
	return &Location{
		Path:            path,
		Scheme:          "http",
		ContainerPath:   "/",
		InjectedConfigs: make([]string, 0),
		Extras:          NewExtrasMap(),
		Containers:      make(map[string]*Container),
	}
}

// AddContainer adds a container to the location
func (l *Location) AddContainer(container *Container) {
	l.Containers[container.ID] = container
	if len(l.Containers) > 1 {
		l.UpstreamEnabled = true
	}
}

// RemoveContainer removes a container from the location
func (l *Location) RemoveContainer(containerID string) bool {
	if _, exists := l.Containers[containerID]; exists {
		delete(l.Containers, containerID)
		if len(l.Containers) <= 1 {
			l.UpstreamEnabled = false
		}
		return true
	}
	return false
}

// GetContainers returns a slice of containers in this location
func (l *Location) GetContainers() []*Container {
	containers := make([]*Container, 0, len(l.Containers))
	for _, c := range l.Containers {
		containers = append(containers, c)
	}
	return containers
}

// IsEmpty returns true if the location has no containers
func (l *Location) IsEmpty() bool {
	return len(l.Containers) == 0
}

// GetUpstreamID returns the upstream ID for this location
func (l *Location) GetUpstreamID(hostname string, port int, index int) string {
	return fmt.Sprintf("%s-%d-%d", hostname, port, index)
}

// RemoveContainer removes a container from all locations in the host
func (h *Host) RemoveContainer(containerID string) bool {
	removed := false
	for _, location := range h.Locations {
		if location.RemoveContainer(containerID) {
			removed = true
		}
	}
	return removed
}

// IsEmpty checks if the host has any locations with containers
func (h *Host) IsEmpty() bool {
	for _, location := range h.Locations {
		if len(location.GetContainers()) > 0 {
			return false
		}
	}
	return true
}
