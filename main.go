package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/client"
	"github.com/rahulshinde/nginx-proxy-go/internal/config"
	"github.com/rahulshinde/nginx-proxy-go/internal/debug"
	"github.com/rahulshinde/nginx-proxy-go/internal/dockerapi"
	"github.com/rahulshinde/nginx-proxy-go/internal/webserver"
)

func main() {
	// Initialize debug configuration
	debugCfg := debug.NewConfig()
	if err := debug.StartDebugServer(debugCfg); err != nil {
		log.Printf("Warning: Debug mode initialization failed: %v", err)
	}

	// Create Docker client
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Load configuration
	cfg := config.NewConfig()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Create web server instance
	server, err := webserver.NewWebServer(dockerapi.New(cli), cfg, nil)
	if err != nil {
		log.Fatalf("Failed to create web server: %v", err)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v, initiating shutdown...\n", sig)
		cancel()
	}()

	// Start the server
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
