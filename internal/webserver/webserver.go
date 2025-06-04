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
	"github.com/rahulshinde/nginx-proxy-go/internal/config"
	"github.com/rahulshinde/nginx-proxy-go/internal/container"
	"github.com/rahulshinde/nginx-proxy-go/internal/errors"
	"github.com/rahulshinde/nginx-proxy-go/internal/event"
	"github.com/rahulshinde/nginx-proxy-go/internal/host"
	"github.com/rahulshinde/nginx-proxy-go/internal/logger"
	"github.com/rahulshinde/nginx-proxy-go/internal/nginx"
	"github.com/rahulshinde/nginx-proxy-go/internal/processor"
)

// WebServer represents the main nginx proxy server
type WebServer struct {
	client             *client.Client
	config             *config.Config
	nginx              *nginx.Nginx
	hosts              map[string]*host.Host
	containers         map[string]*container.Container
	networks           map[string]string
	mu                 sync.RWMutex
	template           *nginx.Template
	basicAuthProcessor *processor.BasicAuthProcessor
	eventProcessor     *event.Processor
	log                *logger.Logger
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

	ws := &WebServer{
		client:             client,
		config:             cfg,
		hosts:              make(map[string]*host.Host),
		containers:         make(map[string]*container.Container),
		networks:           make(map[string]string),
		basicAuthProcessor: processor.NewBasicAuthProcessor(filepath.Join(cfg.ConfDir, "basic_auth")),
		log:                logger,
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

// loadTemplate loads the nginx configuration template
func (ws *WebServer) loadTemplate() (*nginx.Template, error) {
	tmplStr := `map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

proxy_cache off;
proxy_request_buffering off;

{{range .Hosts}}
{{range .Upstreams}}
upstream {{ .ID }} {
    {{range .Containers}}
    server {{ .Address }}:{{ .Port }} max_fails=3 fail_timeout=30s;
    {{end}}
    keepalive 32;
}
{{end}}

{{if .SSLEnabled}}
server {
    server_name {{ .Hostname }};
    listen {{ .Port }} ssl http2 {{if .IsDefaultServer}}default_server{{end}};
    ssl_certificate /etc/ssl/certs/{{ .SSLFile }}.crt;
    ssl_certificate_key /etc/ssl/private/{{ .SSLFile }}.key;

    {{if .IsRedirect}}
    return 301 https://{{ .RedirectHostname }}$request_uri;
    {{else if .IsDown}}
    return 503;
    {{else}}
    {{if .BasicAuth}}
    auth_basic "Basic Auth Enabled";
    auth_basic_user_file {{ .BasicAuthFile }};
    {{end}}

    # Global injected configs
    {{range $key, $value := .Extras.ToMap}}
    {{if eq $key "injected"}}
    {{range $config := $value}}
    {{ $config }};
    {{end}}
    {{else if eq $key "security"}}
    {{range $username, $password := $value}}
    auth_basic_user_file {{ $.BasicAuthFile }};
    {{end}}
    {{else}}
    {{ $key }} {{ $value }};
    {{end}}
    {{end}}

    {{range .Locations}}
    location {{ .Path }} {
        # Location-specific injected configs
        {{range .InjectedConfigs}}
        {{ . }};
        {{end}}

        # Location-specific extras
        {{range $key, $value := .Extras.ToMap}}
        {{if eq $key "injected"}}
        {{range $config := $value}}
        {{ $config }};
        {{end}}
        {{else if eq $key "security"}}
        {{range $username, $password := $value}}
        auth_basic_user_file {{ $.BasicAuthFile }};
        {{end}}
        {{else}}
        {{ $key }} {{ $value }};
        {{end}}
        {{end}}

        {{if .BasicAuth}}
        auth_basic "Basic Auth Enabled";
        auth_basic_user_file {{ .BasicAuthFile }};
        {{end}}

        # Proxy settings
        {{if .UpstreamEnabled}}
        proxy_pass {{ .Scheme }}://{{ .Upstream }}{{ .ContainerPath }};
        {{else}}
        proxy_pass {{ .Scheme }}://{{ .ContainerAddress }}:{{ .ContainerPort }}{{ .ContainerPath }};
        {{end}}

        {{if ne .Path "/"}}
        proxy_redirect $scheme://$http_host{{ .ContainerPath }} $scheme://$http_host{{ .Path }};
        {{end}}

        # WebSocket settings
        {{if and .WebSocket .HTTP}}
        proxy_set_header Host $http_host;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $proxy_x_forwarded_proto;
        proxy_set_header X-Forwarded-Ssl $proxy_x_forwarded_ssl;
        proxy_set_header X-Forwarded-Port $proxy_x_forwarded_port;
        {{else if .WebSocket}}
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        {{end}}
    }
    {{end}}
    {{end}}
}
{{else}}
server {
    listen {{ .Port }} {{if .IsDefaultServer}}default_server{{end}};
    server_name {{ .Hostname }};

    {{if .IsRedirect}}
    return 301 {{if .SSLEnabled}}https{{else}}http{{end}}://{{ .RedirectHostname }}$request_uri;
    {{else}}
    # Global injected configs
    {{range $key, $value := .Extras.ToMap}}
    {{if eq $key "injected"}}
    {{range $config := $value}}
    {{ $config }};
    {{end}}
    {{else if eq $key "security"}}
    {{range $username, $password := $value}}
    auth_basic_user_file {{ $.BasicAuthFile }};
    {{end}}
    {{else}}
    {{ $key }} {{ $value }};
    {{end}}
    {{end}}

    {{range .Locations}}
    location {{ .Path }} {
        # Location-specific injected configs
        {{range .InjectedConfigs}}
        {{ . }};
        {{end}}

        # Location-specific extras
        {{range $key, $value := .Extras.ToMap}}
        {{if eq $key "injected"}}
        {{range $config := $value}}
        {{ $config }};
        {{end}}
        {{else if eq $key "security"}}
        {{range $username, $password := $value}}
        auth_basic_user_file {{ $.BasicAuthFile }};
        {{end}}
        {{else}}
        {{ $key }} {{ $value }};
        {{end}}
        {{end}}

        {{if .BasicAuth}}
        auth_basic "Basic Auth Enabled";
        auth_basic_user_file {{ .BasicAuthFile }};
        {{end}}

        # Proxy settings
        {{if .UpstreamEnabled}}
        proxy_pass {{ .Scheme }}://{{ .Upstream }}{{ .ContainerPath }};
        {{else}}
        proxy_pass {{ .Scheme }}://{{ .ContainerAddress }}:{{ .ContainerPort }}{{ .ContainerPath }};
        {{end}}

        {{if ne .Path "/"}}
        proxy_redirect $scheme://$http_host{{ .ContainerPath }} $scheme://$http_host{{ .Path }};
        {{end}}

        # WebSocket settings
        {{if and .WebSocket .HTTP}}
        proxy_set_header Host $http_host;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $proxy_x_forwarded_proto;
        proxy_set_header X-Forwarded-Ssl $proxy_x_forwarded_ssl;
        proxy_set_header X-Forwarded-Port $proxy_x_forwarded_port;
        {{else if .WebSocket}}
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        {{end}}
    }
    {{end}}
    {{end}}

    location /.well-known/acme-challenge/ {
        alias {{ .Config.ChallengeDir }};
        try_files $uri =404;
    }
}
{{end}}

{{if .SSLRedirect}}
server {
    listen 80 {{if .IsDefaultServer}}default_server{{end}};
    server_name {{ .Hostname }};
    location /.well-known/acme-challenge/ {
        alias {{ .Config.ChallengeDir }};
        try_files $uri =404;
    }
    location / {
        {{if .IsRedirect}}
        return 301 https://{{ .RedirectHostname }}$request_uri;
        {{else}}
        return 301 https://$host$request_uri;
        {{end}}
    }
}
{{end}}
{{end}}

{{if .Config.DefaultServer}}
server {
    listen 80 default_server;
    server_name _;
    location /.well-known/acme-challenge/ {
        alias {{ .Config.ChallengeDir }};
        try_files $uri =404;
    }
    location / {
        return 503;
    }
}
{{end}}`

	return nginx.NewTemplate(tmplStr)
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
	if _, exists := ws.networks[networkID]; exists {
		delete(ws.networks, networkID)
		delete(ws.networks, ws.networks[networkID])
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

	// Get container info
	container, err := ws.client.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return errors.New(errors.ErrorTypeDocker, "failed to inspect container", err).
			WithContext("container_id", containerID)
	}

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
		// Process basic auth
		hostsByPort := make(map[string]map[int]*host.Host)
		for hostname, h := range hosts {
			if _, ok := hostsByPort[hostname]; !ok {
				hostsByPort[hostname] = make(map[int]*host.Host)
			}
			hostsByPort[hostname][h.Port] = h
		}
		ws.basicAuthProcessor.ProcessBasicAuth(env, hostsByPort)

		// Add hosts to the web server
		for hostname, h := range hosts {
			ws.hosts[hostname] = h
		}

		// Reload nginx configuration
		return ws.reload()
	}

	return nil
}

// removeContainer removes a container from the configuration
func (ws *WebServer) removeContainer(containerID string) error {
	// Remove container from all upstreams
	for _, h := range ws.hosts {
		for _, upstream := range h.Upstreams {
			for i, c := range upstream.Containers {
				if c.Address == containerID {
					// Remove container from upstream
					upstream.Containers = append(upstream.Containers[:i], upstream.Containers[i+1:]...)
					break
				}
			}
		}
	}
	return ws.reload()
}

// reload reloads the nginx configuration
func (ws *WebServer) reload() error {
	ws.log.Debug("Reloading nginx configuration...")

	config, err := ws.template.Render(ws.hosts, ws.config)
	if err != nil {
		return errors.New(errors.ErrorTypeConfig, "failed to render template", err)
	}

	if err := ws.nginx.UpdateConfig(config); err != nil {
		return errors.New(errors.ErrorTypeNginx, "failed to update nginx config", err)
	}

	ws.log.Info("Nginx configuration reloaded successfully")
	return nil
}
