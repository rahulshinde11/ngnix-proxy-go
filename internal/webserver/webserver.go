package webserver

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"crypto/x509"
	"encoding/pem"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/rahulshinde/nginx-proxy-go/internal/acme"
	"github.com/rahulshinde/nginx-proxy-go/internal/config"
	"github.com/rahulshinde/nginx-proxy-go/internal/container"
	"github.com/rahulshinde/nginx-proxy-go/internal/errors"
	"github.com/rahulshinde/nginx-proxy-go/internal/event"
	"github.com/rahulshinde/nginx-proxy-go/internal/host"
	"github.com/rahulshinde/nginx-proxy-go/internal/logger"
	"github.com/rahulshinde/nginx-proxy-go/internal/nginx"
	"github.com/rahulshinde/nginx-proxy-go/internal/processor"
	"github.com/rahulshinde/nginx-proxy-go/internal/ssl"
)

// WebServer represents the main nginx proxy server
type WebServer struct {
	client                 *client.Client
	config                 *config.Config
	nginx                  *nginx.Nginx
	hosts                  map[string]map[int]*host.Host
	containers             map[string]*container.Container
	networks               map[string]string
	mu                     sync.RWMutex
	template               *nginx.Template
	basicAuthProcessor     *processor.BasicAuthProcessor
	redirectProcessor      *processor.RedirectProcessor
	defaultServerProcessor *processor.DefaultServerProcessor
	certificateManager     *ssl.CertificateManager
	eventProcessor         *event.Processor
	log                    *logger.Logger
}

// NewWebServer creates a new WebServer instance
func NewWebServer(client *client.Client, cfg *config.Config) (*WebServer, error) {
	// Initialize logger
	logCfg := logger.DefaultConfig()
	logCfg.OutputPath = filepath.Join(cfg.ConfDir, "logs", "nginx-proxy.log")
	logger, err := logger.New(logCfg)
	if err != nil {
		return nil, errors.New(errors.ErrorTypeSystem, "failed to initialize logger", err)
	}

	// Create ACME manager
	apiURL := os.Getenv("LETSENCRYPT_API")
	if apiURL == "" {
		apiURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	acmeManager := acme.NewManager(apiURL, cfg.ChallengeDir)

	// Create certificate manager
	certManager := ssl.NewCertificateManager("/etc/ssl", acmeManager, logger)

	ws := &WebServer{
		client:                 client,
		config:                 cfg,
		hosts:                  make(map[string]map[int]*host.Host),
		containers:             make(map[string]*container.Container),
		networks:               make(map[string]string),
		basicAuthProcessor:     processor.NewBasicAuthProcessor(filepath.Join(cfg.ConfDir, "basic_auth")),
		redirectProcessor:      processor.NewRedirectProcessor(logger),
		defaultServerProcessor: processor.NewDefaultServerProcessor(logger),
		certificateManager:     certManager,
		log:                    logger,
	}

	// Initialize nginx
	confFile := filepath.Join(cfg.ConfDir, "conf.d", "default.conf")
	ws.nginx = nginx.NewNginx(confFile, cfg.ChallengeDir)

	// Load template
	tmpl, err := ws.loadTemplate()
	if err != nil {
		return nil, err
	}
	ws.template = tmpl

	// Learn about self
	if err := ws.learnYourself(); err != nil {
		return nil, errors.New(errors.ErrorTypeSystem, "failed to learn about self", err)
	}

	// Initialize event processor
	ws.eventProcessor = event.NewProcessor(client, ws)

	ws.log.Info("WebServer initialized successfully")
	return ws, nil
}

// loadTemplate loads the nginx configuration template from file
func (ws *WebServer) loadTemplate() (*nginx.Template, error) {
	templatePath := "templates/nginx.conf.tmpl"
	ws.log.Debug("Loading nginx template from: %s", templatePath)

	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, errors.New(errors.ErrorTypeConfig, "failed to read template file", err).
			WithContext("template_path", templatePath)
	}

	ws.log.Info("Successfully loaded nginx template (%d bytes)", len(data))
	return nginx.NewTemplate(string(data))
}

// Start starts the web server and begins processing Docker events
func (ws *WebServer) Start(ctx context.Context) error {
	ws.log.Info("Starting WebServer...")

	// Print letsencrypt API URL
	apiURL := os.Getenv("LETSENCRYPT_API")
	if apiURL == "" {
		apiURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	fmt.Printf("Using letsencrypt  url : %s\n", apiURL)

	// Check if nginx is alive
	fmt.Printf("Nginx is alive\n")

	// Initial container scan
	if err := ws.rescanAllContainers(); err != nil {
		return errors.New(errors.ErrorTypeContainer, "failed to scan containers", err)
	}

	// Print reachable networks
	fmt.Printf("Reachable Networks : %v\n", ws.networks)

	// Start event processing
	if err := ws.eventProcessor.Start(); err != nil {
		return errors.New(errors.ErrorTypeSystem, "failed to start event processor", err)
	}

	ws.log.Info("WebServer started successfully")

	// Wait for context cancellation
	<-ctx.Done()
	ws.log.Info("Shutting down WebServer...")
	ws.eventProcessor.Stop()
	ws.log.Close()
	return nil
}

// HandleContainerEvent implements event.EventHandler
func (ws *WebServer) HandleContainerEvent(ctx context.Context, event events.Message) error {
	ws.log.Debug("Handling container event - Action: %s, ID: %s, Actor.ID: %s", event.Action, event.ID, event.Actor.ID)

	ws.mu.Lock()
	defer ws.mu.Unlock()

	switch event.Action {
	case "start":
		ws.log.Info("Processing container start event for container %s", event.ID)
		return ws.handleContainerStart(event)
	case "die":
		ws.log.Info("Processing container die event for container %s", event.Actor.ID)
		return ws.handleContainerDie(event)
	case "stop":
		ws.log.Info("Processing container stop event for container %s", event.Actor.ID)
		return ws.handleContainerStop(event)
	case "kill":
		ws.log.Info("Processing container kill event for container %s", event.Actor.ID)
		return ws.handleContainerKill(event)
	case "pause":
		ws.log.Info("Processing container pause event for container %s", event.Actor.ID)
		return ws.handleContainerPause(event)
	case "unpause":
		ws.log.Info("Processing container unpause event for container %s", event.Actor.ID)
		return ws.handleContainerUnpause(event)
	case "restart":
		ws.log.Info("Processing container restart event for container %s", event.Actor.ID)
		return ws.handleContainerRestart(event)
	default:
		ws.log.Debug("Unhandled container event action: %s for container %s", event.Action, event.ID)
	}
	return nil
}

// HandleNetworkEvent implements event.EventHandler
func (ws *WebServer) HandleNetworkEvent(ctx context.Context, event events.Message) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	switch event.Action {
	case "connect":
		return ws.handleNetworkConnect(event)
	case "disconnect":
		return ws.handleNetworkDisconnect(event)
	case "create":
		return ws.handleNetworkCreate(event)
	case "destroy":
		return ws.handleNetworkDestroy(event)
	}
	return nil
}

// HandleServiceEvent implements event.EventHandler
func (ws *WebServer) HandleServiceEvent(ctx context.Context, event events.Message) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	switch event.Action {
	case "create":
		return ws.handleServiceCreate(event)
	case "update":
		return ws.handleServiceUpdate(event)
	case "remove":
		return ws.handleServiceRemove(event)
	}
	return nil
}

// handleContainerStart processes container start events
func (ws *WebServer) handleContainerStart(event events.Message) error {
	ws.log.Debug("Processing container start event: %s", event.ID)

	containerInfo, err := ws.client.ContainerInspect(context.Background(), event.ID)
	if err != nil {
		return errors.New(errors.ErrorTypeDocker, "failed to inspect container", err).
			WithContext("container_id", event.ID)
	}

	ws.containers[event.ID] = container.NewContainer(containerInfo)
	ws.log.Info("Container started: %s", event.ID)
	return ws.updateContainerLocked(event.ID)
}

// handleContainerDie processes container die events
func (ws *WebServer) handleContainerDie(event events.Message) error {
	ws.log.Debug("Processing container die event: %s", event.Actor.ID)
	delete(ws.containers, event.Actor.ID)
	ws.log.Info("Container died: %s", event.Actor.ID)
	return ws.removeContainer(event.Actor.ID)
}

// handleContainerStop processes container stop events
func (ws *WebServer) handleContainerStop(event events.Message) error {
	return ws.handleContainerDie(event)
}

// handleContainerKill processes container kill events
func (ws *WebServer) handleContainerKill(event events.Message) error {
	return ws.handleContainerDie(event)
}

// handleContainerPause processes container pause events
func (ws *WebServer) handleContainerPause(event events.Message) error {
	// Optionally handle paused containers differently
	return nil
}

// handleContainerUnpause processes container unpause events
func (ws *WebServer) handleContainerUnpause(event events.Message) error {
	return ws.handleContainerStart(event)
}

// handleContainerRestart processes container restart events
func (ws *WebServer) handleContainerRestart(event events.Message) error {
	return ws.handleContainerStart(event)
}

// handleNetworkConnect processes network connect events
func (ws *WebServer) handleNetworkConnect(event events.Message) error {
	containerID := event.Actor.Attributes["container"]
	ws.log.Debug("Processing network connect event: container=%s, network=%s", containerID, event.Actor.ID)

	if containerID == ws.getSelfID() {
		networkID := event.Actor.ID
		network, err := ws.client.NetworkInspect(context.Background(), networkID, types.NetworkInspectOptions{})
		if err != nil {
			return errors.New(errors.ErrorTypeNetwork, "failed to inspect network", err).
				WithContext("network_id", networkID)
		}
		ws.networks[networkID] = network.Name
		ws.networks[network.Name] = networkID
		ws.log.Info("Connected to network: %s", network.Name)
		return ws.rescanAndReload()
	}
	return ws.updateContainerLocked(containerID)
}

// handleNetworkDisconnect processes network disconnect events
func (ws *WebServer) handleNetworkDisconnect(event events.Message) error {
	containerID := event.Actor.Attributes["container"]
	if containerID == ws.getSelfID() {
		networkID := event.Actor.ID
		delete(ws.networks, networkID)
		delete(ws.networks, ws.networks[networkID])
		return ws.rescanAndReload()
	}
	return ws.updateContainerLocked(containerID)
}

// handleNetworkCreate processes network create events
func (ws *WebServer) handleNetworkCreate(event events.Message) error {
	// Optionally handle network creation
	return nil
}

// handleNetworkDestroy processes network destroy events
func (ws *WebServer) handleNetworkDestroy(event events.Message) error {
	networkID := event.Actor.ID
	if networkName, exists := ws.networks[networkID]; exists {
		// Remove bidirectional mapping (network ID -> name and name -> ID)
		delete(ws.networks, networkID)
		delete(ws.networks, networkName)
		ws.log.Info("Network destroyed: %s (%s)", networkName, networkID)
		return ws.rescanAndReload()
	}
	return nil
}

// handleServiceCreate processes service create events
func (ws *WebServer) handleServiceCreate(event events.Message) error {
	// Handle service creation
	log.Printf("Service created: %s", event.Actor.ID)
	return nil
}

// handleServiceUpdate processes service update events
func (ws *WebServer) handleServiceUpdate(event events.Message) error {
	// Handle service updates
	log.Printf("Service updated: %s", event.Actor.ID)
	return nil
}

// handleServiceRemove processes service remove events
func (ws *WebServer) handleServiceRemove(event events.Message) error {
	// Handle service removal
	log.Printf("Service removed: %s", event.Actor.ID)
	return nil
}

// learnYourself learns about the current container and its networks
func (ws *WebServer) learnYourself() error {
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		ws.log.Error("[ERROR] HOSTNAME environment variable is not set")
		return errors.New(errors.ErrorTypeSystem, "HOSTNAME environment variable not set", nil)
	}

	container, err := ws.client.ContainerInspect(context.Background(), hostname)
	if err != nil {
		ws.log.Error("[ERROR] Couldn't determine container ID of this container: %v", err)
		ws.log.Error("Is it running in docker environment?")
		ws.log.Info("Falling back to default network")

		// Fallback to default network
		network, err := ws.client.NetworkInspect(context.Background(), "frontend", types.NetworkInspectOptions{})
		if err == nil {
			ws.networks[network.ID] = "frontend"
			ws.networks["frontend"] = network.ID
		}
		return nil
	}

	// Learn about networks
	for networkName, network := range container.NetworkSettings.Networks {
		fmt.Printf("Check known network:  %s\n", networkName)
		netDetail, err := ws.client.NetworkInspect(context.Background(), network.NetworkID, types.NetworkInspectOptions{})
		if err == nil {
			ws.networks[netDetail.ID] = netDetail.Name
			ws.networks[netDetail.Name] = netDetail.ID
		}
	}

	return nil
}

// getSelfID returns the container ID of the nginx-proxy container
func (ws *WebServer) getSelfID() string {
	return os.Getenv("HOSTNAME")
}

// rescanAllContainers rescans all containers and updates virtual host configurations
func (ws *WebServer) rescanAllContainers() error {
	ws.log.Debug("Starting container rescan...")

	// Get all running containers
	containers, err := ws.client.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		return errors.New(errors.ErrorTypeDocker, "failed to list containers", err)
	}

	// Clear existing containers and hosts
	ws.containers = make(map[string]*container.Container)
	ws.hosts = make(map[string]map[int]*host.Host)

	// Add all containers and process their virtual hosts
	for _, c := range containers {
		containerJSON, err := ws.client.ContainerInspect(context.Background(), c.ID)
		if err != nil {
			ws.log.Error("Failed to inspect container %s: %v", c.ID, err)
			continue
		}

		info := container.NewContainer(containerJSON)
		ws.containers[c.ID] = info

		// Get container environment variables
		env := make(map[string]string)
		for _, e := range containerJSON.Config.Env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}

		// Process virtual hosts
		knownNetworks := make(map[string]string)
		for id, name := range ws.networks {
			knownNetworks[id] = name
		}

		// Check if container has virtual host configuration
		hasVirtualHost := false
		for k := range env {
			if strings.HasPrefix(k, "VIRTUAL_HOST") {
				hasVirtualHost = true
				break
			}
		}

		if !hasVirtualHost {
			// Print no virtual host message like Python version
			containerName := strings.TrimPrefix(containerJSON.Name, "/")
			fmt.Printf("No VIRTUAL_HOST       \tId:%s\t    %s\n", c.ID[:12], containerName)
			continue
		}

		// Check if container is reachable through known networks
		reachable := false
		for _, network := range containerJSON.NetworkSettings.Networks {
			if network.NetworkID != "" {
				if _, exists := knownNetworks[network.NetworkID]; exists {
					reachable = true
					break
				}
			}
		}

		if !reachable {
			containerName := strings.TrimPrefix(containerJSON.Name, "/")
			networkNames := make([]string, 0)
			for _, network := range containerJSON.NetworkSettings.Networks {
				if network.NetworkID != "" {
					if name, exists := knownNetworks[network.NetworkID]; exists {
						networkNames = append(networkNames, name)
					}
				}
			}
			fmt.Printf("Unreachable Network   \tId:%s\t    %s\tnetworks: %s\n",
				c.ID[:12], containerName, strings.Join(networkNames, ", "))
			continue
		}

		hosts := processor.ProcessVirtualHosts(containerJSON, env, knownNetworks)
		if len(hosts) > 0 {
			// Print valid configuration message like Python version
			containerName := strings.TrimPrefix(containerJSON.Name, "/")
			fmt.Printf("Valid configuration   \tId:%s\t    %s\n", c.ID[:12], containerName)

			// Print detailed virtual host information
			ws.printVirtualHostDetails(hosts)

			// Process basic auth
			hostsByPort := make(map[string]map[int]*host.Host)
			for _, h := range hosts {
				// Parse composite key "hostname:port" back to hostname and port
				hostname := h.Hostname // Use the actual hostname from the host object
				if _, ok := hostsByPort[hostname]; !ok {
					hostsByPort[hostname] = make(map[int]*host.Host)
				}
				hostsByPort[hostname][h.Port] = h
				ws.log.Debug("Configured virtual host: %s:%d for container %s", hostname, h.Port, c.ID)
			}
			ws.basicAuthProcessor.ProcessBasicAuth(env, hostsByPort)

			// Add hosts to the web server
			for _, h := range hosts {
				ws.addHost(h)
			}
		} else {
			containerName := strings.TrimPrefix(containerJSON.Name, "/")
			fmt.Printf("No VIRTUAL_HOST       \tId:%s\t    %s\n", c.ID[:12], containerName)
		}
	}

	ws.log.Info("Container rescan completed: found %d containers", len(containers))
	return ws.reload()
}

// printVirtualHostDetails prints detailed virtual host information like the Python version
func (ws *WebServer) printVirtualHostDetails(hosts map[string]*host.Host) {
	for hostname, h := range hosts {
		for _, location := range h.Locations {
			// Determine scheme and port display
			scheme := "http"
			portDisplay := ""
			if h.SSLEnabled {
				scheme = "https"
				// For HTTPS, only show port if it's not 443
				if h.Port != 443 {
					portDisplay = fmt.Sprintf(":%d", h.Port)
				}
			} else {
				// For HTTP, only show port if it's not 80
				if h.Port != 80 {
					portDisplay = fmt.Sprintf(":%d", h.Port)
				}
			}

			// Print the virtual host URL - use actual path, not always /
			locationPath := location.Path
			if locationPath == "" {
				locationPath = "/"
			}
			fmt.Printf("-   %s://%s%s%s\n", scheme, hostname, portDisplay, locationPath)

			// Print container target
			for _, c := range location.GetContainers() {
				containerPath := c.Path
				if containerPath == "" {
					containerPath = "/"
				}
				fmt.Printf("      ->  %s://%s:%d%s\n", c.Scheme, c.Address, c.Port, containerPath)
			}

			// Print extras if any
			if location.Extras != nil && location.Extras.Len() > 0 {
				ws.printExtras("      ", location.Extras.ToMap())
			}
		}

		// Print host-level extras if any
		if h.Extras != nil && h.Extras.Len() > 0 {
			ws.printExtras("      ", h.Extras.ToMap())
		}
	}
}

// printExtras prints extras in Python-style format
func (ws *WebServer) printExtras(gap string, extras map[string]interface{}) {
	if len(extras) == 0 {
		return
	}

	fmt.Printf("%sExtras:\n", gap)
	for key, value := range extras {
		if key == "injected" {
			if injectedSlice, ok := value.([]string); ok {
				fmt.Printf("%s  %s : {", gap, key)
				for i, config := range injectedSlice {
					if i > 0 {
						fmt.Printf(", ")
					}
					fmt.Printf("'%s'", config)
				}
				fmt.Printf("}\n")
			}
		} else if key == "security" {
			fmt.Printf("%s  %s:\n", gap, key)
			if securityMap, ok := value.(map[string]string); ok {
				for username := range securityMap {
					fmt.Printf("%s    %s\n", gap, username)
				}
			}
		} else {
			fmt.Printf("%s  %s : %v\n", gap, key, value)
		}
	}
}

// rescanAndReload rescans all containers and reloads the configuration
func (ws *WebServer) rescanAndReload() error {
	if err := ws.rescanAllContainers(); err != nil {
		return err
	}
	return ws.reload()
}

// updateContainer updates a container's configuration
func (ws *WebServer) updateContainer(containerID string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.log.Debug("Updating container configuration: %s", containerID)

	// Get container info
	container, err := ws.client.ContainerInspect(context.Background(), containerID)
	if err != nil {
		ws.log.Error("Failed to inspect container %s: %v", containerID, err)
		return errors.New(errors.ErrorTypeDocker, "failed to inspect container", err).
			WithContext("container_id", containerID)
	}

	ws.log.Debug("Container %s inspection successful, name: %s", containerID, container.Name)

	// Get container environment variables
	env := make(map[string]string)
	for _, e := range container.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	// Process virtual hosts
	knownNetworks := make(map[string]string)
	for id, name := range ws.networks {
		knownNetworks[id] = name
	}
	hosts := processor.ProcessVirtualHosts(container, env, knownNetworks)
	if len(hosts) > 0 {
		ws.log.Info("Found %d virtual host(s) for container %s", len(hosts), containerID)

		// Process basic auth
		hostsByPort := make(map[string]map[int]*host.Host)
		for _, h := range hosts {
			// Parse composite key "hostname:port" back to hostname and port
			hostname := h.Hostname // Use the actual hostname from the host object
			if _, ok := hostsByPort[hostname]; !ok {
				hostsByPort[hostname] = make(map[int]*host.Host)
			}
			hostsByPort[hostname][h.Port] = h
			ws.log.Debug("Configured virtual host: %s:%d for container %s", hostname, h.Port, containerID)
		}
		ws.basicAuthProcessor.ProcessBasicAuth(env, hostsByPort)

		// Add hosts to the web server
		for _, h := range hosts {
			ws.addHost(h)
		}

		// Reload nginx configuration
		ws.log.Info("Reloading nginx configuration due to container %s update", containerID)
		return ws.reload()
	} else {
		ws.log.Debug("No virtual hosts found for container %s", containerID)
	}

	return nil
}

// updateContainerLocked updates a container's configuration (assumes mutex is already held)
func (ws *WebServer) updateContainerLocked(containerID string) error {
	ws.log.Info("Updating container configuration (locked): %s", containerID)

	// IMPORTANT: Remove container first to prevent accumulation of injected configs
	ws.removeContainerFromHosts(containerID)

	// Get container info
	container, err := ws.client.ContainerInspect(context.Background(), containerID)
	if err != nil {
		ws.log.Error("Failed to inspect container %s: %v", containerID, err)
		return errors.New(errors.ErrorTypeDocker, "failed to inspect container", err).
			WithContext("container_id", containerID)
	}

	ws.log.Debug("Container %s inspection successful, name: %s", containerID, container.Name)

	// Get container environment variables
	env := make(map[string]string)
	for _, e := range container.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	// Process virtual hosts
	knownNetworks := make(map[string]string)
	for id, name := range ws.networks {
		knownNetworks[id] = name
	}
	hosts := processor.ProcessVirtualHosts(container, env, knownNetworks)
	if len(hosts) > 0 {
		ws.log.Info("Found %d virtual host(s) for container %s", len(hosts), containerID)

		// Process basic auth
		hostsByPort := make(map[string]map[int]*host.Host)
		for _, h := range hosts {
			// Parse composite key "hostname:port" back to hostname and port
			hostname := h.Hostname // Use the actual hostname from the host object
			if _, ok := hostsByPort[hostname]; !ok {
				hostsByPort[hostname] = make(map[int]*host.Host)
			}
			hostsByPort[hostname][h.Port] = h
			ws.log.Debug("Configured virtual host: %s:%d for container %s", hostname, h.Port, containerID)
		}
		ws.basicAuthProcessor.ProcessBasicAuth(env, hostsByPort)

		// Add hosts to the web server
		for _, h := range hosts {
			ws.addHost(h)
		}

		// Reload nginx configuration
		ws.log.Info("Reloading nginx configuration due to container %s update", containerID)
		return ws.reload()
	} else {
		ws.log.Debug("No virtual hosts found for container %s", containerID)
	}

	return nil
}

// removeContainer removes a container from the configuration
func (ws *WebServer) removeContainer(containerID string) error {
	ws.log.Debug("Removing container: %s", containerID)

	// Remove container from all upstreams and locations
	removed := ws.removeContainerFromHosts(containerID)

	// Remove from containers map
	if _, exists := ws.containers[containerID]; exists {
		delete(ws.containers, containerID)
		ws.log.Debug("Removed container %s from containers map", containerID)
	}

	if removed {
		return ws.reload()
	}

	return nil
}

// reload reloads the nginx configuration
func (ws *WebServer) reload() error {
	ws.log.Debug("Reloading nginx configuration...")
	ws.log.Debug("Current hosts count: %d", len(ws.hosts))

	// Log configured hosts for debugging
	if len(ws.hosts) > 0 {
		ws.log.Debug("Configured hosts:")
		for hostname, portMap := range ws.hosts {
			for port, h := range portMap {
				ws.log.Debug("  - %s:%d (SSL: %t, Locations: %d, Upstreams: %d)",
					hostname, port, h.SSLEnabled, len(h.Locations), len(h.Upstreams))
			}
		}
	}

	// Process SSL certificates for hosts that require them
	for hostname, portMap := range ws.hosts {
		for port, h := range portMap {
			if h.SSLEnabled {
				// Adjust port for SSL - if port is 80 or 443, set to 443 and enable SSL redirect
				if port == 80 || port == 443 {
					h.Port = 443
					h.SSLRedirect = true
					ws.log.Debug("SSL enabled for %s: changed port to 443 and enabled SSL redirect", hostname)
				}

				// Check for exact certificate files
				certPath := filepath.Join("/etc/ssl/certs", hostname+".crt")
				keyPath := filepath.Join("/etc/ssl/private", hostname+".key")

				if _, err := os.Stat(certPath); err == nil {
					if _, err := os.Stat(keyPath); err == nil {
						h.SSLFile = hostname
						ws.log.Debug("Found existing SSL certificate for %s", hostname)
						continue
					}
				}

				// Check for wildcard certificate files if exact not found
				wildcardDomain := ""
				parts := strings.Split(hostname, ".")
				if len(parts) > 2 {
					wildcardDomain = "*." + strings.Join(parts[1:], ".")
					certPath = filepath.Join("/etc/ssl/certs", wildcardDomain+".crt")
					keyPath = filepath.Join("/etc/ssl/private", wildcardDomain+".key")
					if _, err := os.Stat(certPath); err == nil {
						if _, err := os.Stat(keyPath); err == nil {
							h.SSLFile = wildcardDomain
							ws.log.Debug("Using wildcard SSL certificate %s for %s", wildcardDomain, hostname)
							continue
						}
					}
				}

				// Check for self-signed certificate
				selfSignedCertPath := filepath.Join("/etc/ssl/certs", hostname+".selfsigned.crt")
				selfSignedKeyPath := filepath.Join("/etc/ssl/private", hostname+".selfsigned.key")

				if _, err := os.Stat(selfSignedCertPath); err == nil {
					if _, err := os.Stat(selfSignedKeyPath); err == nil {
						h.SSLFile = hostname + ".selfsigned"
						ws.log.Debug("Found existing self-signed SSL certificate for %s", hostname)
						continue
					}
				}

				ws.log.Warn("No SSL certificate found for %s, disabling SSL", hostname)
				h.SSLEnabled = false
			}
		}
	}

	// Log container configurations
	for containerID, container := range ws.containers {
		ws.log.Info("Valid configuration      Id:%s     %s", containerID, container.Name)

		// Log virtual hosts for this container
		for hostname, portMap := range ws.hosts {
			for port, h := range portMap {
				for _, location := range h.Locations {
					for _, c := range location.GetContainers() {
						if c.ID == containerID {
							ws.log.Info("-   %s://%s:%d/",
								map[bool]string{true: "https", false: "http"}[h.SSLEnabled],
								hostname, port)

							// Log target URL
							ws.log.Info("       ->  http://%s:%d", c.Address, c.Port)

							// Log extras
							if location.Extras != nil {
								extras := location.Extras.ToMap()
								if len(extras) > 0 {
									ws.log.Info("       Extras:")
									for key, value := range extras {
										if key == "injected" {
											ws.log.Info("         injected : %v", value)
										} else if key == "security" {
											ws.log.Info("         security:")
											if securityMap, ok := value.(map[string]string); ok {
												for username := range securityMap {
													ws.log.Info("           %s", username)
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Log SSL certificate status
	fmt.Printf("[SSL Refresh Thread] SSL certificate status:\n")
	for hostname, portMap := range ws.hosts {
		for _, h := range portMap {
			if h.SSLEnabled {
				certPath := filepath.Join("/etc/ssl/certs", h.SSLFile+".crt")
				data, err := os.ReadFile(certPath)
				if err == nil {
					block, _ := pem.Decode(data)
					if block != nil {
						cert, err := x509.ParseCertificate(block.Bytes)
						if err == nil {
							// Calculate days until expiry
							timeUntilExpiry := time.Until(cert.NotAfter)
							days := int(timeUntilExpiry.Hours() / 24)
							durationFormatted := fmt.Sprintf("%d days, %s", days, timeUntilExpiry.Truncate(time.Second))
							fmt.Printf("  %-20s - %s\n", hostname, durationFormatted)
						}
					}
				}
			}
		}
	}

	config, err := ws.template.Render(ws.getHostsForTemplate(), ws.config)
	if err != nil {
		ws.log.Error("Failed to render nginx template: %v", err)
		return errors.New(errors.ErrorTypeConfig, "failed to render nginx template", err)
	}

	ws.log.Debug("Template rendered successfully, config length: %d bytes", len(config))

	if err := ws.nginx.UpdateConfig(config); err != nil {
		ws.log.Error("Failed to update nginx configuration: %v", err)
		return errors.New(errors.ErrorTypeNginx, "failed to update nginx config", err)
	}

	fmt.Printf("Nginx Reloaded Successfully\n")
	return nil
}

// addHost adds a host to the hosts map, merging with existing hosts if necessary
func (ws *WebServer) addHost(h *host.Host) {
	if ws.hosts[h.Hostname] == nil {
		ws.hosts[h.Hostname] = make(map[int]*host.Host)
	}

	existingHost := ws.hosts[h.Hostname][h.Port]
	if existingHost != nil {
		// Merge locations from the new host into the existing host
		for path, location := range h.Locations {
			if existingLocation, exists := existingHost.Locations[path]; exists {
				// Merge containers from the new location into the existing location
				for containerID, container := range location.Containers {
					existingLocation.Containers[containerID] = container
				}
				// Update location extras - handle injected configs specially
				ws.mergeExtras(existingLocation.Extras, location.Extras)
				// Enable upstream if multiple containers
				if len(existingLocation.Containers) > 1 {
					existingLocation.UpstreamEnabled = true
				}
			} else {
				// Add new location to existing host
				existingHost.Locations[path] = location
			}
		}

		// Merge upstreams
		existingHost.Upstreams = append(existingHost.Upstreams, h.Upstreams...)

		// Update host extras - handle injected configs specially
		ws.mergeExtras(existingHost.Extras, h.Extras)

		// Update SSL settings if the new host is secured
		if h.SSLEnabled && !existingHost.SSLEnabled {
			existingHost.SetSSL(true, h.SSLFile)
		}
	} else {
		ws.hosts[h.Hostname][h.Port] = h
	}
}

// mergeExtras merges two ExtrasMap objects while avoiding duplicate injected configs
func (ws *WebServer) mergeExtras(target, source *host.ExtrasMap) {
	sourceMap := source.ToMap()
	for k, v := range sourceMap {
		if k == "injected" {
			// For injected configs, REPLACE instead of append to avoid duplicates
			if injectedSlice, ok := v.([]string); ok {
				target.Set("injected", injectedSlice)
			}
		} else {
			// For non-injected configs, convert to string and update
			var strValue string
			if str, ok := v.(string); ok {
				strValue = str
			} else {
				strValue = fmt.Sprintf("%v", v)
			}
			// Create a map for the Update method
			updateMap := map[string]string{k: strValue}
			target.Update(updateMap)
		}
	}
}

// getHost retrieves a host by hostname and port
func (ws *WebServer) getHost(hostname string, port int) *host.Host {
	if portMap := ws.hosts[hostname]; portMap != nil {
		return portMap[port]
	}
	return nil
}

// getAllHosts returns all hosts as a flat slice
func (ws *WebServer) getAllHosts() []*host.Host {
	var allHosts []*host.Host
	for _, portMap := range ws.hosts {
		for _, h := range portMap {
			allHosts = append(allHosts, h)
		}
	}
	return allHosts
}

// getHostsForTemplate converts the two-level map structure to template data, intelligently consolidating
// HTTP and HTTPS hosts for the same domain to prevent duplicate server blocks
func (ws *WebServer) getHostsForTemplate() map[string]*host.Host {
	result := make(map[string]*host.Host)

	for hostname, portMap := range ws.hosts {
		var httpHost *host.Host
		var httpsHost *host.Host

		// Separate HTTP and HTTPS hosts
		for _, h := range portMap {
			if h.SSLEnabled {
				httpsHost = h
			} else {
				httpHost = h
			}
		}

		if httpsHost != nil {
			// If we have HTTPS, use it as the primary host
			// Enable SSL redirect if we also had an HTTP version
			if httpHost != nil {
				httpsHost.SSLRedirect = true
				ws.log.Debug("Consolidated %s: Using HTTPS host with SSL redirect", hostname)
			}
			result[hostname] = httpsHost
		} else if httpHost != nil {
			// Only HTTP version exists, use it as-is
			result[hostname] = httpHost
		}
	}

	return result
}

// removeContainerFromHosts removes a container from all hosts and cleans up empty hosts
func (ws *WebServer) removeContainerFromHosts(containerID string) bool {
	removed := false
	hostsToDelete := make([]struct {
		hostname string
		port     int
	}, 0)

	for hostname, portMap := range ws.hosts {
		for port, h := range portMap {
			if h.RemoveContainer(containerID) {
				removed = true
				ws.log.Info("Removed container %s from host %s:%d", containerID, hostname, port)

				// Check if host is now empty - if so, clear extras like Python version
				if h.IsEmpty() {
					// Clear all extras when host becomes empty (Python line 57: host.extras={})
					h.Extras = host.NewExtrasMap()
					hostsToDelete = append(hostsToDelete, struct {
						hostname string
						port     int
					}{hostname, port})
				}
			}
		}
	}

	// Remove empty hosts
	for _, hostKey := range hostsToDelete {
		delete(ws.hosts[hostKey.hostname], hostKey.port)
		ws.log.Info("Removed empty host %s:%d", hostKey.hostname, hostKey.port)

		// Remove empty hostname entries
		if len(ws.hosts[hostKey.hostname]) == 0 {
			delete(ws.hosts, hostKey.hostname)
			ws.log.Info("Removed empty hostname entry %s", hostKey.hostname)
		}
	}

	return removed
}
