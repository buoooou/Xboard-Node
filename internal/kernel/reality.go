package kernel

import (
	"strings"

	"github.com/cedar2025/xboard-node/internal/model"
)

// ApplyRealityDest returns nc with its Reality fallback target replaced by
// dest (kernel.reality_dest) when the node uses Reality and dest is set.
// Otherwise nc is returned unchanged. nc itself is never modified.
//
// The panel only sends reality_settings.server_name, from which both kernels
// derive the target as server_name:443. When server_name is a domain that
// resolves to this machine, that target is the node's own listener and the
// fallback loops; pointing dest at a local web server avoids that.
func ApplyRealityDest(nc *model.NodeSpec, dest string) *model.NodeSpec {
	dest = strings.TrimSpace(dest)
	if nc == nil || dest == "" || nc.TLS != 2 {
		return nc
	}
	clone := *nc
	settings := make(map[string]any, len(nc.TLSSettings)+1)
	for k, v := range nc.TLSSettings {
		settings[k] = v
	}
	settings["dest"] = dest
	clone.TLSSettings = settings
	return &clone
}
