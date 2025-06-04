package debug

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Config holds debug configuration
type Config struct {
	Enabled bool
	Port    int
	Host    string
}

// NewConfig creates a new debug configuration from environment variables
func NewConfig() *Config {
	enabled := getEnvBool("GO_DEBUG_ENABLE", false)
	port := getEnvInt("GO_DEBUG_PORT", 2345) // Default Delve port
	host := getEnv("GO_DEBUG_HOST", "")

	// If host is not specified, try to get the default network interface IP
	if enabled && host == "" {
		host = getDefaultInterfaceIP()
	}

	return &Config{
		Enabled: enabled,
		Port:    port,
		Host:    host,
	}
}

// StartDebugServer starts the Delve debug server if debug mode is enabled
func StartDebugServer(cfg *Config) error {
	if !cfg.Enabled {
		return nil
	}

	log.Printf("Starting nginx-proxy in debug mode. Debug server will listen on %s:%d", cfg.Host, cfg.Port)

	// Check if Delve is installed
	if _, err := exec.LookPath("dlv"); err != nil {
		return fmt.Errorf("delve debugger not found. Please install it with: go install github.com/go-delve/delve/cmd/dlv@latest")
	}

	// Start Delve in headless mode
	cmd := exec.Command("dlv", "debug",
		"--headless",
		"--listen", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		"--api-version", "2",
		"--accept-multiclient",
		"--continue",
		"--output", "nginx-proxy-debug")

	// Set up pipes for output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the debug server
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start debug server: %v", err)
	}

	return nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvBool gets a boolean environment variable
func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		boolValue, err := strconv.ParseBool(value)
		if err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getEnvInt gets an integer environment variable
func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		intValue, err := strconv.Atoi(value)
		if err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getDefaultInterfaceIP returns the IP address of the default network interface
func getDefaultInterfaceIP() string {
	// Try to get the default route interface
	cmd := exec.Command("ip", "route")
	output, err := cmd.Output()
	if err != nil {
		return "127.0.0.1"
	}

	// Parse the output to get the default interface
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "default via") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				// Get the interface name
				ifaceName := fields[4]

				// Get the IP address of the interface
				iface, err := net.InterfaceByName(ifaceName)
				if err != nil {
					continue
				}

				addrs, err := iface.Addrs()
				if err != nil {
					continue
				}

				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
						if ipnet.IP.To4() != nil {
							return ipnet.IP.String()
						}
					}
				}
			}
		}
	}

	return "127.0.0.1"
}
