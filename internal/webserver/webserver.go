package webserver

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	hosts                  map[string]*host.Host
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
		hosts:                  make(map[string]*host.Host),
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
		return nil, errors.New(errors.ErrorTypeConfig, "failed to load template", err)
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

	// Initial container scan
	if err := ws.rescanAllContainers(); err != nil {
		return errors.New(errors.ErrorTypeContainer, "failed to scan containers", err)
	}

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
	ws.mu.Lock()
	defer ws.mu.Unlock()

	switch event.Action {
	case "start":
		return ws.handleContainerStart(event)
	case "die":
		return ws.handleContainerDie(event)
	case "stop":
		return ws.handleContainerStop(event)
	case "kill":
		return ws.handleContainerKill(event)
	case "pause":
		return ws.handleContainerPause(event)
	case "unpause":
		return ws.handleContainerUnpause(event)
	case "restart":
		return ws.handleContainerRestart(event)
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
	return ws.rescanAndReload()
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
	return ws.updateContainer(containerID)
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
	return ws.updateContainer(containerID)
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

// learnYourself determines the container ID and networks of the nginx-proxy container
func (ws *WebServer) learnYourself() error {
	ws.log.Debug("Learning about self...")

	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		// In development, use a default hostname
		hostname = "nginx-proxy-dev"
		ws.log.Info("No HOSTNAME set, using development hostname: %s", hostname)
	}

	// Try to inspect the container
	container, err := ws.client.ContainerInspect(context.Background(), hostname)
	if err != nil {
		// In development, we can continue without container inspection
		ws.log.Warn("Failed to inspect container (this is normal in development): %v", err)
		return nil
	}

	// If we're in a container, learn about networks
	for network := range container.NetworkSettings.Networks {
		net, err := ws.client.NetworkInspect(context.Background(), network, types.NetworkInspectOptions{})
		if err != nil {
			return errors.New(errors.ErrorTypeNetwork, "failed to inspect network", err).
				WithContext("network", network)
		}
		ws.networks[net.ID] = net.Name
		ws.networks[net.Name] = net.ID
		ws.log.Debug("Connected to network: %s", net.Name)
	}

	ws.log.Info("Self discovery completed: container=%s, networks=%v", hostname, ws.networks)
	return nil
}

// getSelfID returns the container ID of the nginx-proxy container
func (ws *WebServer) getSelfID() string {
	return os.Getenv("HOSTNAME")
}

// rescanAllContainers rescans all containers and updates the configuration
func (ws *WebServer) rescanAllContainers() error {
	ws.log.Debug("Rescanning all containers...")

	containers, err := ws.client.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		return errors.New(errors.ErrorTypeDocker, "failed to list containers", err)
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Clear existing containers
	ws.containers = make(map[string]*container.Container)

	// Add all containers
	for _, c := range containers {
		containerInfo, err := ws.client.ContainerInspect(context.Background(), c.ID)
		if err != nil {
			return errors.New(errors.ErrorTypeDocker, "failed to inspect container", err).
				WithContext("container_id", c.ID)
		}
		ws.containers[c.ID] = container.NewContainer(containerInfo)
		ws.log.Debug("Found container: %s", c.ID)
	}

	ws.log.Info("Container rescan completed: found %d containers", len(containers))
	return ws.reload()
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
		for hostname, h := range hosts {
			if _, ok := hostsByPort[hostname]; !ok {
				hostsByPort[hostname] = make(map[int]*host.Host)
			}
			hostsByPort[hostname][h.Port] = h
			ws.log.Debug("Configured virtual host: %s:%d for container %s", hostname, h.Port, containerID)
		}
		ws.basicAuthProcessor.ProcessBasicAuth(env, hostsByPort)

		// Add hosts to the web server
		for hostname, h := range hosts {
			ws.hosts[hostname] = h
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
	removed := false
	for hostname, h := range ws.hosts {
		// Remove from upstreams
		for i, upstream := range h.Upstreams {
			for j, c := range upstream.Containers {
				if c.ID == containerID { // Fixed: Compare container ID, not IP address
					// Remove container from upstream
					upstream.Containers = append(upstream.Containers[:j], upstream.Containers[j+1:]...)
					ws.log.Info("Removed container %s from upstream %s of host %s", containerID, upstream.ID, hostname)
					removed = true
				}
			}

			// Remove empty upstreams
			if len(upstream.Containers) == 0 {
				h.Upstreams = append(h.Upstreams[:i], h.Upstreams[i+1:]...)
				ws.log.Info("Removed empty upstream %s from host %s", upstream.ID, hostname)
			}
		}

		// Remove from locations
		for path, location := range h.Locations {
			if location.RemoveContainer(containerID) {
				ws.log.Info("Removed container %s from location %s of host %s", containerID, path, hostname)
				removed = true
			}

			// Remove empty locations
			if location.IsEmpty() {
				delete(h.Locations, path)
				ws.log.Info("Removed empty location %s from host %s", path, hostname)
			}
		}

		// Remove empty hosts
		if len(h.Upstreams) == 0 && len(h.Locations) == 0 {
			delete(ws.hosts, hostname)
			ws.log.Info("Removed empty host %s", hostname)
		}
	}

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
		for hostname, h := range ws.hosts {
			ws.log.Debug("  - %s:%d (SSL: %t, Locations: %d, Upstreams: %d)",
				hostname, h.Port, h.SSLEnabled, len(h.Locations), len(h.Upstreams))
		}
	}

	config, err := ws.template.Render(ws.hosts, ws.config)
	if err != nil {
		ws.log.Error("Failed to render nginx template: %v", err)
		return errors.New(errors.ErrorTypeConfig, "failed to render template", err)
	}

	ws.log.Debug("Template rendered successfully, config length: %d bytes", len(config))

	if err := ws.nginx.UpdateConfig(config); err != nil {
		ws.log.Error("Failed to update nginx configuration: %v", err)
		return errors.New(errors.ErrorTypeNginx, "failed to update nginx config", err)
	}

	ws.log.Info("Nginx configuration reloaded successfully")
	return nil
}
