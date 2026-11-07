package singbox

import (
	"encoding/json"
	"testing"

	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/kernel"
	"github.com/cedar2025/xboard-node/internal/panel"
)

func realityHandshake(t *testing.T, kcfg config.KernelConfig, nc *panel.NodeConfig) (map[string]any, map[string]any) {
	t.Helper()
	cfg := buildConfig(kcfg, testNodeSpec(nc), testUsers, kernel.TLSCert{})
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	tls := parsed["inbounds"].([]any)[0].(map[string]any)["tls"].(map[string]any)
	return tls, tls["reality"].(map[string]any)["handshake"].(map[string]any)
}

func selfStealNode(protocol string) *panel.NodeConfig {
	return &panel.NodeConfig{
		Protocol:   protocol,
		ServerPort: 443,
		TLS:        2,
		TLSSettings: map[string]any{
			"private_key": "pk",
			"short_id":    "abc123",
			"server_name": "n1.example.com",
		},
	}
}

func TestBuildConfig_RealityDestOverride(t *testing.T) {
	for _, protocol := range []string{"vless", "trojan"} {
		kcfg := config.KernelConfig{Type: "singbox", LogLevel: "warn", RealityDest: "127.0.0.1:8443"}
		nc := selfStealNode(protocol)
		tls, handshake := realityHandshake(t, kcfg, nc)
		if handshake["server"] != "127.0.0.1" || handshake["server_port"] != float64(8443) {
			t.Errorf("%s: handshake = %v, want 127.0.0.1:8443", protocol, handshake)
		}
		if tls["server_name"] != "n1.example.com" {
			t.Errorf("%s: server_name = %v, want n1.example.com", protocol, tls["server_name"])
		}
		if _, ok := nc.TLSSettings["dest"]; ok {
			t.Errorf("%s: panel tls_settings was mutated", protocol)
		}
	}
}

func TestBuildConfig_RealityDestOverride_IPv6(t *testing.T) {
	kcfg := config.KernelConfig{Type: "singbox", LogLevel: "warn", RealityDest: "[::1]:8443"}
	_, handshake := realityHandshake(t, kcfg, selfStealNode("vless"))
	if handshake["server"] != "::1" || handshake["server_port"] != float64(8443) {
		t.Errorf("handshake = %v, want [::1]:8443", handshake)
	}
}

func TestBuildConfig_RealityDestUnset(t *testing.T) {
	kcfg := config.KernelConfig{Type: "singbox", LogLevel: "warn"}
	_, handshake := realityHandshake(t, kcfg, selfStealNode("vless"))
	if handshake["server"] != "n1.example.com" || handshake["server_port"] != float64(443) {
		t.Errorf("handshake = %v, want n1.example.com:443", handshake)
	}
}
