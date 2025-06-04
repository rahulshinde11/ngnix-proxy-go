package host

import (
	"fmt"
)

// ProxyConfigData represents a central aggregator for host configurations
type ProxyConfigData struct {
	// Map of hostname -> port -> host configuration
	configMap map[string]map[int]*Host
	// Set of container IDs
	containers map[string]struct{}
	// Length of unique host configurations
	length int
}

// NewProxyConfigData creates a new ProxyConfigData instance
func NewProxyConfigData() *ProxyConfigData {
	return &ProxyConfigData{
		configMap:  make(map[string]map[int]*Host),
		containers: make(map[string]struct{}),
	}
}

// GetHost returns a host configuration for the given hostname and port
func (p *ProxyConfigData) GetHost(hostname string, port int) *Host {
	if portMap, ok := p.configMap[hostname]; ok {
		if host, ok := portMap[port]; ok {
			return host
		}
	}
	return nil
}

// AddHost adds or merges a host configuration
func (p *ProxyConfigData) AddHost(host *Host) {
	if portMap, ok := p.configMap[host.Hostname]; ok {
		if existingHost, ok := portMap[host.Port]; ok {
			// Merge with existing host
			existingHost.SSLEnabled = existingHost.SSLEnabled || host.SSLEnabled
			existingHost.UpdateExtras(host.Extras.ToMap())

			// Merge locations
			for path, location := range host.Locations {
				for _, container := range location.GetContainers() {
					existingHost.AddLocation(path, container, nil)
					p.containers[container.ID] = struct{}{}
				}
				if existingLoc, ok := existingHost.Locations[path]; ok {
					existingLoc.UpdateExtras(location.Extras.ToMap())
				}
			}
			return
		} else {
			p.length++
			portMap[host.Port] = host
		}
	} else {
		p.length++
		p.configMap[host.Hostname] = map[int]*Host{host.Port: host}
	}

	// Add containers to set
	for _, location := range host.Locations {
		for _, container := range location.GetContainers() {
			p.containers[container.ID] = struct{}{}
		}
	}
}

// RemoveContainer removes a container from all host configurations
func (p *ProxyConfigData) RemoveContainer(containerID string) (bool, map[string]int) {
	removedDomains := make(map[string]int)
	result := false

	if _, exists := p.containers[containerID]; exists {
		delete(p.containers, containerID)

		// Remove container from all hosts
		for hostname, portMap := range p.configMap {
			for port, host := range portMap {
				if host.RemoveContainer(containerID) {
					result = true
					if host.IsEmpty() {
						host.Extras = NewExtrasMap()
						removedDomains[hostname] = port
					}
				}
			}
		}
	}

	return result, removedDomains
}

// HasContainer checks if a container exists in the configuration
func (p *ProxyConfigData) HasContainer(containerID string) bool {
	_, exists := p.containers[containerID]
	return exists
}

// HostList returns a channel of all host configurations
func (p *ProxyConfigData) HostList() []*Host {
	var hosts []*Host
	for _, portMap := range p.configMap {
		for _, host := range portMap {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// Len returns the number of unique host configurations
func (p *ProxyConfigData) Len() int {
	return p.length
}

// Print prints the current configuration state
func (p *ProxyConfigData) Print() {
	for _, host := range p.HostList() {
		postfix := "://" + host.Hostname
		hostURL := func(isWebsocket bool) string {
			scheme := "http"
			if isWebsocket {
				scheme = "ws"
			}
			if host.SSLEnabled {
				scheme += "s"
			}
			port := ""
			if (host.SSLEnabled && host.Port != 443) || (!host.SSLEnabled && host.Port != 80) {
				port = fmt.Sprintf(":%d", host.Port)
			}
			return fmt.Sprintf("-   %s%s%s", scheme, postfix, port)
		}

		if host.IsRedirect {
			fmt.Println(hostURL(false))
			fmt.Printf("      redirect : %s\n", host.RedirectHostname)
		} else {
			if host.Extras.Len() > 0 {
				fmt.Println(hostURL(false))
				p.printExtras("      ", host.Extras.ToMap())
			}
			for _, location := range host.Locations {
				fmt.Println(hostURL(location.WebSocket) + location.Path)
				for _, container := range location.GetContainers() {
					port := ""
					if container.Port != 0 {
						port = fmt.Sprintf(":%d", container.Port)
					}
					fmt.Printf("      -> %s://%s%s%s\n", container.Scheme, container.Address, port, location.ContainerPath)
				}
				if location.Extras.Len() > 0 {
					p.printExtras("      ", location.Extras.ToMap())
				}
			}
		}
	}
}

// printExtras prints extras in a formatted way
func (p *ProxyConfigData) printExtras(gap string, extras map[string]interface{}) {
	fmt.Println(gap + "Extras:")
	for k, v := range extras {
		switch val := v.(type) {
		case map[string]interface{}:
			fmt.Printf("%s  %s:\n", gap, k)
			for sk, sv := range val {
				fmt.Printf("%s    %s:%v\n", gap, sk, sv)
			}
		case []interface{}:
			fmt.Printf("%s  %s:\n", gap, k)
			for _, sv := range val {
				fmt.Printf("%s    %v\n", gap, sv)
			}
		default:
			fmt.Printf("%s  %s : %v\n", gap, k, v)
		}
	}
}
