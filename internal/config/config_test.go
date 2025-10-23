package config

import (
	"os"
	"testing"
)

func TestNewConfigDefaults(t *testing.T) {
	clearEnv(t)

	cfg := NewConfig()

	if cfg.ConfDir != "./nginx/" {
		t.Fatalf("ConfDir: expected ./nginx/, got %s", cfg.ConfDir)
	}
	if cfg.ChallengeDir != "./acme-challenges/" {
		t.Fatalf("ChallengeDir: expected ./acme-challenges/, got %s", cfg.ChallengeDir)
	}
	if cfg.SSLDir != "./ssl/" {
		t.Fatalf("SSLDir: expected ./ssl/, got %s", cfg.SSLDir)
	}
	if cfg.ClientMaxBodySize != "1m" {
		t.Fatalf("ClientMaxBodySize: expected 1m, got %s", cfg.ClientMaxBodySize)
	}
	if !cfg.DefaultServer {
		t.Fatalf("DefaultServer: expected true")
	}
	if cfg.BasicAuthFile != "/etc/nginx/basic_auth" {
		t.Fatalf("BasicAuthFile: expected /etc/nginx/basic_auth, got %s", cfg.BasicAuthFile)
	}
	if cfg.DebugEnabled {
		t.Fatalf("DebugEnabled: expected false")
	}
	if cfg.DebugPort != 2345 {
		t.Fatalf("DebugPort: expected 2345, got %d", cfg.DebugPort)
	}
}

func TestNewConfigEnvOverrides(t *testing.T) {
	clearEnv(t)
	os.Setenv("NGINX_CONF_DIR", "/tmp/nginx")
	os.Setenv("CHALLENGE_DIR", "/tmp/chal")
	os.Setenv("SSL_DIR", "/tmp/ssl")
	os.Setenv("CLIENT_MAX_BODY_SIZE", "10m")
	os.Setenv("DEFAULT_HOST", "false")
	os.Setenv("GO_DEBUG_ENABLE", "true")
	os.Setenv("GO_DEBUG_PORT", "9000")
	os.Setenv("GO_DEBUG_HOST", "127.0.0.1")

	cfg := NewConfig()

	if cfg.ConfDir != "/tmp/nginx/" {
		t.Fatalf("ConfDir: expected /tmp/nginx/, got %s", cfg.ConfDir)
	}
	if cfg.ChallengeDir != "/tmp/chal/" {
		t.Fatalf("ChallengeDir: expected /tmp/chal/, got %s", cfg.ChallengeDir)
	}
	if cfg.SSLDir != "/tmp/ssl/" {
		t.Fatalf("SSLDir: expected /tmp/ssl/, got %s", cfg.SSLDir)
	}
	if cfg.ClientMaxBodySize != "10m" {
		t.Fatalf("ClientMaxBodySize: expected 10m, got %s", cfg.ClientMaxBodySize)
	}
	if cfg.DefaultServer {
		t.Fatalf("DefaultServer: expected false")
	}
	if !cfg.DebugEnabled {
		t.Fatalf("DebugEnabled: expected true")
	}
	if cfg.DebugPort != 9000 {
		t.Fatalf("DebugPort: expected 9000, got %d", cfg.DebugPort)
	}
	if cfg.DebugHost != "127.0.0.1" {
		t.Fatalf("DebugHost: expected 127.0.0.1, got %s", cfg.DebugHost)
	}
}

func TestEnsureTrailingSlash(t *testing.T) {
	cases := map[string]string{
		"/tmp":      "/tmp/",
		"/tmp/":     "/tmp/",
		"relative":  "relative/",
		"relative/": "relative/",
		"":          "/",
		"./example": "./example/",
	}

	for input, want := range cases {
		if got := ensureTrailingSlash(input); got != want {
			t.Errorf("ensureTrailingSlash(%q) = %q, want %q", input, got, want)
		}
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"NGINX_CONF_DIR",
		"CHALLENGE_DIR",
		"SSL_DIR",
		"CLIENT_MAX_BODY_SIZE",
		"DEFAULT_HOST",
		"GO_DEBUG_ENABLE",
		"GO_DEBUG_PORT",
		"GO_DEBUG_HOST",
	}
	for _, k := range keys {
		os.Unsetenv(k)
	}
}
