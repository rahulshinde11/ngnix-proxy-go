package host

import "testing"

func TestParseVirtualHost_BasicHTTP(t *testing.T) {
	config, err := ParseVirtualHost("example.com")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Hostname != "example.com" {
		t.Fatalf("expected hostname example.com, got %s", config.Hostname)
	}
	if config.ServerPort != 80 {
		t.Fatalf("expected server port 80, got %d", config.ServerPort)
	}
	if config.Scheme != "http" {
		t.Fatalf("expected scheme http, got %s", config.Scheme)
	}
	if config.Path != "/" {
		t.Fatalf("expected path /, got %s", config.Path)
	}
}

func TestParseVirtualHost_HTTPSWithPortAndPath(t *testing.T) {
	config, err := ParseVirtualHost("https://example.com:443/api -> :8080/internal")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if config.Scheme != "https" {
		t.Fatalf("expected external scheme https, got %s", config.Scheme)
	}
	if config.ServerPort != 443 {
		t.Fatalf("expected server port 443, got %d", config.ServerPort)
	}
	if config.Path != "/internal" {
		t.Fatalf("expected container path /internal, got %s", config.Path)
	}
	if config.ContainerPort != 8080 {
		t.Fatalf("expected container port 8080, got %d", config.ContainerPort)
	}
	if config.ContainerScheme != "http" {
		t.Fatalf("expected container scheme http default, got %s", config.ContainerScheme)
	}
}

func TestParseVirtualHost_WithExtras(t *testing.T) {
	config, err := ParseVirtualHost("example.com -> :8000; proxy_read_timeout=90s; add_header X-Test value")
	if err != nil {
		t.Fatalf("ParseVirtualHost error: %v", err)
	}

	if len(config.Extras) != 2 {
		t.Fatalf("expected 2 extras, got %d", len(config.Extras))
	}
	if config.Extras[0] != "proxy_read_timeout=90s" {
		t.Fatalf("unexpected first extra: %s", config.Extras[0])
	}
	if config.Extras[1] != "add_header X-Test value" {
		t.Fatalf("unexpected second extra: %s", config.Extras[1])
	}
}

func TestParseVirtualHost_InvalidFormat(t *testing.T) {
	_, err := ParseVirtualHost("example.com -> -> :8080")
	if err == nil {
		t.Fatalf("expected error for invalid format")
	}
}
