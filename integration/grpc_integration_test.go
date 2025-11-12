//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/rahulshinde/nginx-proxy-go/internal/host"
)

func TestParseVirtualHostGRPCIntegration(t *testing.T) {
	// Test basic gRPC parsing
	config, err := host.ParseVirtualHost("grpc://api.example.com -> :50051")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Scheme != "grpc" {
		t.Fatalf("expected scheme grpc, got %s", config.Scheme)
	}

	if config.Hostname != "api.example.com" {
		t.Fatalf("expected hostname api.example.com, got %s", config.Hostname)
	}

	if config.ContainerPort != 50051 {
		t.Fatalf("expected container port 50051, got %d", config.ContainerPort)
	}
}

func TestParseVirtualHostGRPCSSIntegration(t *testing.T) {
	// Test secure gRPC parsing
	config, err := host.ParseVirtualHost("grpcs://secure.api.example.com -> :50051")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Scheme != "grpcs" {
		t.Fatalf("expected scheme grpcs, got %s", config.Scheme)
	}

	if config.Hostname != "secure.api.example.com" {
		t.Fatalf("expected hostname secure.api.example.com, got %s", config.Hostname)
	}

	if config.ContainerPort != 50051 {
		t.Fatalf("expected container port 50051, got %d", config.ContainerPort)
	}
}

func TestParseVirtualHostGRPCSSLContainerScheme(t *testing.T) {
	// Test gRPC with SSL container scheme
	config, err := host.ParseVirtualHost("grpc://api.example.com -> grpcs://:50051")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Scheme != "grpc" {
		t.Fatalf("expected external scheme grpc, got %s", config.Scheme)
	}

	if config.ContainerScheme != "grpcs" {
		t.Fatalf("expected container scheme grpcs, got %s", config.ContainerScheme)
	}

	if config.ContainerPort != 50051 {
		t.Fatalf("expected container port 50051, got %d", config.ContainerPort)
	}
}

func TestParseVirtualHostGRPCWithPorts(t *testing.T) {
	// Test gRPC with custom ports and container paths
	config, err := host.ParseVirtualHost("grpc://api.example.com:8443/v1 -> :50051/grpc")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Scheme != "grpc" {
		t.Fatalf("expected scheme grpc, got %s", config.Scheme)
	}

	if config.ServerPort != 8443 {
		t.Fatalf("expected server port 8443, got %d", config.ServerPort)
	}

	if config.Path != "/grpc" {
		t.Fatalf("expected container path /grpc, got %s", config.Path)
	}
}

func TestParseVirtualHostGRPCSubRoutesHTTPSamePort(t *testing.T) {
	// Test sharing port 80 for both HTTP and gRPC services
	config, err := host.ParseVirtualHost("grpc://api.example.com/v1/users -> :80")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Scheme != "grpc" {
		t.Fatalf("expected scheme grpc, got %s", config.Scheme)
	}

	if config.ServerPort != 80 {
		t.Fatalf("expected server port 80, got %d", config.ServerPort)
	}

	if config.ContainerPort != 80 {
		t.Fatalf("expected container port 80, got %d", config.ContainerPort)
	}

	// Sub-routes should work correctly
	if config.Path != "/v1/users" {
		t.Fatalf("expected path /v1/users, got %s", config.Path)
	}
}
