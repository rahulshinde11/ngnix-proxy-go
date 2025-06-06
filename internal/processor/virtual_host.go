package processor

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/rahulshinde/nginx-proxy-go/internal/container"
	"github.com/rahulshinde/nginx-proxy-go/internal/host"
)

// VirtualHostProcessor handles virtual host configuration
type VirtualHostProcessor struct {
	dockerClient *client.Client
	knownNets    []string
	ctx          context.Context
}

// NewVirtualHostProcessor creates a new VirtualHostProcessor
func NewVirtualHostProcessor(dockerClient *client.Client, knownNets []string) *VirtualHostProcessor {
	return &VirtualHostProcessor{
		dockerClient: dockerClient,
		knownNets:    knownNets,
		ctx:          context.Background(),
	}
}

// Process processes container information and returns virtual host configurations
func (p *VirtualHostProcessor) Process(cont types.Container) ([]*host.Host, error) {
	var hosts []*host.Host

	// Get container info
	containerJSON, err := p.dockerClient.ContainerInspect(p.ctx, cont.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %v", err)
	}

	info := container.NewContainer(containerJSON)

	// Convert knownNets slice to map for reachability check
	knownNetsMap := make(map[string]string, len(p.knownNets))
	for _, net := range p.knownNets {
		knownNetsMap[net] = ""
	}

	// Check if container is reachable through known networks
	if !info.IsReachable(knownNetsMap) {
		log.Printf("Container %s is not reachable through known networks, skipping", info.Name)
		return nil, nil
	}

	// Get virtual host configurations
	virtualHosts := make([]string, 0)
	for k, v := range info.Environment {
		if strings.HasPrefix(k, "VIRTUAL_HOST") || strings.HasPrefix(k, "STATIC_VIRTUAL_HOST") {
			virtualHosts = append(virtualHosts, v)
		}
	}

	if len(virtualHosts) == 0 {
		return nil, nil
	}

	// Process each virtual host
	for _, vh := range virtualHosts {
		config, err := host.ParseVirtualHost(vh)
		if err != nil {
			log.Printf("Error parsing virtual host %s: %v", vh, err)
			continue
		}

		// Create host configuration
		h := host.NewHost(config.Hostname, config.ServerPort)
		h.IsStatic = strings.HasPrefix(vh, "STATIC_VIRTUAL_HOST")
		h.Scheme = config.Scheme

		// Set SSL if enabled (for https or wss schemes)
		if config.Scheme == "https" || config.Scheme == "wss" {
			h.SetSSL(true, config.Hostname)
		}

		// Convert extras to map
		extrasMap := make(map[string]string)

		// Process extras - these are nginx config directives that should be injected
		if len(config.Extras) > 0 {
			// Store the extras as injected config directives
			for i, extra := range config.Extras {
				extra = strings.TrimSpace(extra)
				if extra != "" {
					// Check if it's a key=value pair (for known settings)
					if parts := strings.SplitN(extra, "=", 2); len(parts) == 2 {
						extrasMap[parts[0]] = parts[1]
					} else {
						// This is a nginx config directive to be injected
						extrasMap[fmt.Sprintf("injected_%d", i)] = extra
					}
				}
			}
		}

		// Add default values if not present in extras
		if _, ok := extrasMap["websocket"]; !ok {
			extrasMap["websocket"] = "false"
		}
		if _, ok := extrasMap["http"]; !ok {
			extrasMap["http"] = "true"
		}
		if _, ok := extrasMap["scheme"]; !ok {
			extrasMap["scheme"] = config.ContainerScheme
		}
		if _, ok := extrasMap["container_path"]; !ok {
			extrasMap["container_path"] = config.Path
		}

		// Add location
		containerPort := config.ContainerPort
		if containerPort == 0 {
			containerPort = info.Port // Use container's exposed port as fallback
		}
		container := &host.Container{
			ID:      info.ID,
			Address: info.IPAddress,
			Port:    containerPort,
			Scheme:  config.ContainerScheme,
			Path:    config.Path,
		}
		h.AddLocation(config.Path, container, extrasMap)

		// Add upstream
		h.AddUpstream(info.ID, []*host.Container{container})

		hosts = append(hosts, h)
	}

	return hosts, nil
}

// ProcessStaticHosts processes static virtual host configurations
func (p *VirtualHostProcessor) ProcessStaticHosts(staticHosts []string) ([]*host.Host, error) {
	var hosts []*host.Host

	for _, vh := range staticHosts {
		config, err := host.ParseVirtualHost(vh)
		if err != nil {
			log.Printf("Error parsing static virtual host %s: %v", vh, err)
			continue
		}

		// Create host configuration
		h := host.NewHost(config.Hostname, config.ServerPort)
		h.IsStatic = true
		h.Scheme = config.Scheme

		// Set SSL if enabled (for https or wss schemes)
		if config.Scheme == "https" || config.Scheme == "wss" {
			h.SetSSL(true, config.Hostname)
		}

		// Convert extras to map
		extrasMap := make(map[string]string)

		// Process extras - these are nginx config directives that should be injected
		if len(config.Extras) > 0 {
			// Store the extras as injected config directives
			for i, extra := range config.Extras {
				extra = strings.TrimSpace(extra)
				if extra != "" {
					// Check if it's a key=value pair (for known settings)
					if parts := strings.SplitN(extra, "=", 2); len(parts) == 2 {
						extrasMap[parts[0]] = parts[1]
					} else {
						// This is a nginx config directive to be injected
						extrasMap[fmt.Sprintf("injected_%d", i)] = extra
					}
				}
			}
		}

		// Add location
		container := &host.Container{
			ID:      "",
			Address: "",
			Port:    0,
			Scheme:  "",
			Path:    config.Path,
		}
		h.AddLocation(config.Path, container, extrasMap)

		hosts = append(hosts, h)
	}

	return hosts, nil
}

// ProcessVirtualHosts processes virtual host configurations from container environment variables
func ProcessVirtualHosts(container types.ContainerJSON, env map[string]string, knownNetworks map[string]string) map[string]*host.Host {
	hosts := make(map[string]*host.Host)

	// Get virtual host configurations
	virtualHosts := make([]string, 0)
	staticHosts := make([]string, 0)
	for k, v := range env {
		if strings.HasPrefix(k, "VIRTUAL_HOST") {
			virtualHosts = append(virtualHosts, v)
		} else if strings.HasPrefix(k, "STATIC_VIRTUAL_HOST") {
			staticHosts = append(staticHosts, v)
		}
	}

	if len(virtualHosts) == 0 && len(staticHosts) == 0 {
		return hosts
	}

	// Get container IP address from known networks
	var containerIP string
	for _, network := range container.NetworkSettings.Networks {
		if network.NetworkID != "" {
			if _, exists := knownNetworks[network.NetworkID]; exists {
				containerIP = network.IPAddress
				break
			}
		}
	}

	if containerIP == "" {
		return hosts
	}

	// Process static hosts first
	for _, hostConfig := range staticHosts {
		h, location, containerData, extras := parseHostEntry(hostConfig)
		if h == nil {
			continue
		}
		containerData.ID = container.ID
		containerData.Address = containerIP

		// Set default ports based on scheme
		if containerData.Port == 0 {
			if containerData.Scheme == "https" || containerData.Scheme == "wss" {
				containerData.Port = 443
			} else {
				containerData.Port = 80
			}
		}

		// Set SSL based on scheme
		h.SSLEnabled = h.Scheme == "https" || h.Scheme == "wss" || h.Port == 443
		if h.Port == 0 {
			h.Port = 443
		}

		// Add container to host using composite key
		hostKey := fmt.Sprintf("%s:%d", h.Hostname, h.Port)
		h.AddLocation(location, containerData, extras)
		hosts[hostKey] = h
	}

	// Check for SSL and port overrides
	overrideSSL := false
	overridePort := ""
	if len(virtualHosts) == 1 {
		if _, ok := env["LETSENCRYPT_HOST"]; ok {
			overrideSSL = true
		}
		if port, ok := env["VIRTUAL_PORT"]; ok {
			overridePort = port
		}
	}

	// Process virtual hosts
	for _, hostConfig := range virtualHosts {
		h, location, containerData, extras := parseHostEntry(hostConfig)
		if h == nil {
			continue
		}
		containerData.ID = container.ID
		containerData.Address = containerIP

		// Apply port override
		if overridePort != "" {
			port, err := strconv.Atoi(overridePort)
			if err == nil {
				containerData.Port = port
			}
		} else if containerData.Port == 0 {
			// Use exposed ports or default
			if len(container.Config.ExposedPorts) == 1 {
				for port := range container.Config.ExposedPorts {
					portStr := strings.Split(string(port), "/")[0]
					if port, err := strconv.Atoi(portStr); err == nil {
						containerData.Port = port
						break
					}
				}
			} else {
				containerData.Port = 80
			}
		}

		// Apply SSL override
		if overrideSSL {
			if strings.Contains(h.Scheme, "ws") {
				h.Scheme = "wss"
				h.SSLEnabled = true
			} else {
				h.Scheme = "https"
				h.SSLEnabled = true
			}
		}

		// Set SSL based on scheme and port - CRITICAL: Adjust port for HTTPS (Python line 142)
		h.SSLEnabled = h.SSLEnabled || h.Scheme == "https" || h.Scheme == "wss" || h.Port == 443

		// IMPORTANT: Convert HTTPS hosts to port 443 (like Python version does)
		if h.SSLEnabled && h.Port == 80 {
			h.Port = 443
		}

		if h.Port == 0 {
			h.Port = 443
		}

		// Use composite key to handle multiple virtual hosts for same hostname:port
		hostKey := fmt.Sprintf("%s:%d", h.Hostname, h.Port)

		if existingHost, exists := hosts[hostKey]; exists {
			// Merge with existing host - add container to same location
			existingHost.AddLocation(location, containerData, extras)
			// Update SSL if new host is secured
			if h.SSLEnabled && !existingHost.SSLEnabled {
				existingHost.SetSSL(true, h.SSLFile)
			}
		} else {
			// Add container to new host
			h.AddLocation(location, containerData, extras)
			hosts[hostKey] = h
		}
	}

	return hosts
}

// parseHostEntry parses a host entry and returns the host, location, container data, and extras
func parseHostEntry(hostConfig string) (*host.Host, string, *host.Container, map[string]string) {
	config, err := host.ParseVirtualHost(hostConfig)
	if err != nil {
		log.Printf("Error parsing virtual host %s: %v", hostConfig, err)
		return nil, "", nil, nil
	}

	// Create host configuration
	h := host.NewHost(config.Hostname, config.ServerPort)
	h.IsStatic = strings.HasPrefix(hostConfig, "STATIC_VIRTUAL_HOST")
	h.Scheme = config.Scheme

	// Set SSL if enabled (for https or wss schemes)
	if config.Scheme == "https" || config.Scheme == "wss" {
		h.SetSSL(true, config.Hostname)
	}

	// Convert extras to map - follow Python approach more closely
	extrasMap := make(map[string]string)

	// Process extras - store them simply like Python does
	if len(config.Extras) > 0 {
		for i, extra := range config.Extras {
			extra = strings.TrimSpace(extra)
			if extra != "" {
				// Check if it's a key=value pair (for known settings)
				if parts := strings.SplitN(extra, "=", 2); len(parts) == 2 {
					extrasMap[parts[0]] = parts[1]
				} else {
					// This is a nginx config directive to be injected
					// Use indexed keys like before, but handle deduplication in merging
					extrasMap[fmt.Sprintf("injected_%d", i)] = extra
				}
			}
		}
	}

	// Add location
	location := config.Path
	if location == "" {
		location = "/"
	}

	// Create container data
	containerData := &host.Container{
		Address: config.Hostname,
		Port:    config.ContainerPort,
		Scheme:  config.ContainerScheme,
		Path:    location,
	}

	return h, location, containerData, extrasMap
}
