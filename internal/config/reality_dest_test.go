package config

import "testing"

func TestValidateRealityDest(t *testing.T) {
	valid := []string{"", "127.0.0.1:8443", "[::1]:8443", "localhost:443", " 127.0.0.1:8443 "}
	for _, v := range valid {
		if err := ValidateRealityDest(v); err != nil {
			t.Errorf("ValidateRealityDest(%q) = %v, want nil", v, err)
		}
	}
	invalid := []string{"127.0.0.1", "n1.example.com", ":8443", "127.0.0.1:0", "127.0.0.1:70000", "127.0.0.1:https", "::1:8443"}
	for _, v := range invalid {
		if err := ValidateRealityDest(v); err == nil {
			t.Errorf("ValidateRealityDest(%q) = nil, want error", v)
		}
	}
}

func TestExpandNodes_RealityDest(t *testing.T) {
	c := &Config{
		Kernel: KernelConfig{ConfigDir: "/etc/xboard-node", RealityDest: "127.0.0.1:8443"},
		Nodes: []NodeEntry{
			{NodeID: 1},
			{NodeID: 2, Kernel: &KernelOverride{RealityDest: "127.0.0.1:9443"}},
		},
	}
	nodes := c.ExpandNodes()
	if got := nodes[0].Kernel.RealityDest; got != "127.0.0.1:8443" {
		t.Errorf("node 1 reality_dest = %q, want inherited 127.0.0.1:8443", got)
	}
	if got := nodes[1].Kernel.RealityDest; got != "127.0.0.1:9443" {
		t.Errorf("node 2 reality_dest = %q, want override 127.0.0.1:9443", got)
	}
}
