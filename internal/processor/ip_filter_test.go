package processor

import (
	"testing"

	"github.com/rahulshinde/nginx-proxy-go/internal/config"
	"github.com/rahulshinde/nginx-proxy-go/internal/host"
	"github.com/rahulshinde/nginx-proxy-go/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestIPFilterProcessor(trustedIPs []string, header, recursive string) *IPFilterProcessor {
	cfg := &config.Config{
		TrustedProxyIPs: trustedIPs,
		RealIPHeader:    header,
		RealIPRecursive: recursive,
	}
	logCfg := logger.DefaultConfig()
	logCfg.OutputPath = ""
	log, _ := logger.New(logCfg)
	return NewIPFilterProcessor(cfg, log)
}

// --- ParseAndValidateCIDRs tests ---

func TestParseAndValidateCIDRs_ValidCIDRs(t *testing.T) {
	result, err := ParseAndValidateCIDRs("10.0.0.0/8,172.16.0.0/12,192.168.0.0/16")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}, result)
}

func TestParseAndValidateCIDRs_SingleIP_IPv4(t *testing.T) {
	result, err := ParseAndValidateCIDRs("1.2.3.4")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.2.3.4/32"}, result)
}

func TestParseAndValidateCIDRs_SingleIP_IPv6(t *testing.T) {
	result, err := ParseAndValidateCIDRs("2001:db8::1")
	require.NoError(t, err)
	assert.Equal(t, []string{"2001:db8::1/128"}, result)
}

func TestParseAndValidateCIDRs_MixedCIDRAndBareIP(t *testing.T) {
	result, err := ParseAndValidateCIDRs("10.0.0.0/8, 1.2.3.4")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "1.2.3.4/32"}, result)
}

func TestParseAndValidateCIDRs_EmptyInput(t *testing.T) {
	result, err := ParseAndValidateCIDRs("")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseAndValidateCIDRs_WhitespaceOnly(t *testing.T) {
	result, err := ParseAndValidateCIDRs("   ")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseAndValidateCIDRs_WhitespaceAroundEntries(t *testing.T) {
	result, err := ParseAndValidateCIDRs("  10.0.0.0/8 , 172.16.0.0/12  ")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "172.16.0.0/12"}, result)
}

func TestParseAndValidateCIDRs_InvalidCIDR(t *testing.T) {
	result, err := ParseAndValidateCIDRs("not-a-cidr")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestParseAndValidateCIDRs_InvalidMixedWithValid(t *testing.T) {
	result, err := ParseAndValidateCIDRs("10.0.0.0/8,garbage,172.16.0.0/12")
	// Should still return valid entries, no error since we have valid results
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "172.16.0.0/12"}, result)
}

func TestParseAndValidateCIDRs_AllInvalid(t *testing.T) {
	result, err := ParseAndValidateCIDRs("garbage,nonsense")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestParseAndValidateCIDRs_TrailingComma(t *testing.T) {
	result, err := ParseAndValidateCIDRs("10.0.0.0/8,")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8"}, result)
}

// --- ProcessIPFilter tests ---

func TestProcessIPFilter_GlobalConfig(t *testing.T) {
	proc := newTestIPFilterProcessor(
		[]string{"173.245.48.0/20", "103.21.244.0/22"},
		"CF-Connecting-IP",
		"on",
	)

	h := host.NewHost("example.com", 80)
	hosts := map[string]map[int]*host.Host{
		"example.com": {80: h},
	}

	proc.ProcessIPFilter(map[string]string{}, hosts)

	assert.True(t, h.IPFilterEnabled)
	assert.Equal(t, []string{"173.245.48.0/20", "103.21.244.0/22"}, h.AllowedIPs)
	assert.True(t, h.DenyAll)
	assert.Equal(t, "CF-Connecting-IP", h.RealIPHeader)
	assert.Equal(t, "on", h.RealIPRecursive)
}

func TestProcessIPFilter_PerContainerOverride(t *testing.T) {
	proc := newTestIPFilterProcessor(
		[]string{"10.0.0.0/8"},
		"X-Real-IP",
		"on",
	)

	h := host.NewHost("example.com", 80)
	hosts := map[string]map[int]*host.Host{
		"example.com": {80: h},
	}

	env := map[string]string{
		"PROXY_TRUSTED_IPS":    "192.168.1.0/24",
		"PROXY_REAL_IP_HEADER": "X-Forwarded-For",
	}
	proc.ProcessIPFilter(env, hosts)

	assert.True(t, h.IPFilterEnabled)
	assert.Equal(t, []string{"192.168.1.0/24"}, h.AllowedIPs)
	assert.True(t, h.DenyAll)
	assert.Equal(t, "X-Forwarded-For", h.RealIPHeader)
}

func TestProcessIPFilter_NoConfig(t *testing.T) {
	proc := newTestIPFilterProcessor(nil, "", "on")

	h := host.NewHost("example.com", 80)
	hosts := map[string]map[int]*host.Host{
		"example.com": {80: h},
	}

	proc.ProcessIPFilter(map[string]string{}, hosts)

	assert.False(t, h.IPFilterEnabled)
	assert.Nil(t, h.AllowedIPs)
	assert.False(t, h.DenyAll)
}

func TestProcessIPFilter_GlobalIPsNoHeader(t *testing.T) {
	proc := newTestIPFilterProcessor(
		[]string{"10.0.0.0/8"},
		"", // no header
		"on",
	)

	h := host.NewHost("example.com", 80)
	hosts := map[string]map[int]*host.Host{
		"example.com": {80: h},
	}

	proc.ProcessIPFilter(map[string]string{}, hosts)

	assert.True(t, h.IPFilterEnabled)
	assert.Equal(t, []string{"10.0.0.0/8"}, h.AllowedIPs)
	assert.True(t, h.DenyAll)
	assert.Equal(t, "", h.RealIPHeader) // no real IP header
}

func TestProcessIPFilter_MultipleHosts(t *testing.T) {
	proc := newTestIPFilterProcessor(
		[]string{"10.0.0.0/8"},
		"X-Real-IP",
		"on",
	)

	h1 := host.NewHost("a.example.com", 80)
	h2 := host.NewHost("b.example.com", 443)
	hosts := map[string]map[int]*host.Host{
		"a.example.com": {80: h1},
		"b.example.com": {443: h2},
	}

	proc.ProcessIPFilter(map[string]string{}, hosts)

	assert.True(t, h1.IPFilterEnabled)
	assert.True(t, h2.IPFilterEnabled)
	assert.Equal(t, []string{"10.0.0.0/8"}, h1.AllowedIPs)
	assert.Equal(t, []string{"10.0.0.0/8"}, h2.AllowedIPs)
}

func TestProcessIPFilter_PerContainerOverridesGlobalCompletely(t *testing.T) {
	proc := newTestIPFilterProcessor(
		[]string{"10.0.0.0/8", "172.16.0.0/12"},
		"X-Real-IP",
		"on",
	)

	h := host.NewHost("example.com", 80)
	hosts := map[string]map[int]*host.Host{
		"example.com": {80: h},
	}

	// Per-container sets only one IP, should fully replace global
	env := map[string]string{
		"PROXY_TRUSTED_IPS": "192.168.1.0/24",
	}
	proc.ProcessIPFilter(env, hosts)

	assert.Equal(t, []string{"192.168.1.0/24"}, h.AllowedIPs)
	// Header should fall back to global since not overridden
	assert.Equal(t, "X-Real-IP", h.RealIPHeader)
}
