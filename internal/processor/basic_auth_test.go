package processor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rahulshinde/nginx-proxy-go/internal/host"
)

func TestBasicAuthProcessor_ProcessBasicAuth(t *testing.T) {
	dir := t.TempDir()
	proc := NewBasicAuthProcessor(dir)

	h := host.NewHost("example.com", 80)
	hosts := map[string]map[int]*host.Host{
		"example.com": {
			80: h,
		},
	}

	env := map[string]string{
		"PROXY_BASIC_AUTH": "https://example.com -> user:pass",
	}

	if err := proc.ProcessBasicAuth(env, hosts); err != nil {
		t.Fatalf("ProcessBasicAuth error: %v", err)
	}

	credFile := filepath.Join(dir, "example.com.htpasswd")
	if _, err := os.Stat(credFile); err != nil {
		t.Fatalf("expected credential file: %v", err)
	}

	if !h.BasicAuth {
		t.Fatalf("expected host basic auth enabled")
	}
	if h.BasicAuthFile != credFile {
		t.Fatalf("expected auth file %s, got %s", credFile, h.BasicAuthFile)
	}
}
