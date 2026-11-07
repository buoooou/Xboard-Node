package xray

import (
	"encoding/json"
	"testing"

	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/kernel"
	"github.com/cedar2025/xboard-node/internal/panel"
)

func realitySettings(t *testing.T, kcfg config.KernelConfig, nc *panel.NodeConfig) map[string]any {
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
	ib := parsed["inbounds"].([]any)[0].(map[string]any)
	return ib["streamSettings"].(map[string]any)["realitySettings"].(map[string]any)
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
		kcfg := testKernelCfg
		kcfg.RealityDest = "127.0.0.1:8443"
		nc := selfStealNode(protocol)
		rs := realitySettings(t, kcfg, nc)
		if rs["dest"] != "127.0.0.1:8443" {
			t.Errorf("%s: dest = %v, want 127.0.0.1:8443", protocol, rs["dest"])
		}
		names, _ := rs["serverNames"].([]any)
		if len(names) != 1 || names[0] != "n1.example.com" {
			t.Errorf("%s: serverNames = %v, want [n1.example.com]", protocol, rs["serverNames"])
		}
		if _, ok := nc.TLSSettings["dest"]; ok {
			t.Errorf("%s: panel tls_settings was mutated", protocol)
		}
	}
}

func TestBuildConfig_RealityDestUnset(t *testing.T) {
	rs := realitySettings(t, testKernelCfg, selfStealNode("vless"))
	if rs["dest"] != "n1.example.com:443" {
		t.Errorf("dest = %v, want n1.example.com:443", rs["dest"])
	}
}
