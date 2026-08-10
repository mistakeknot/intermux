// Package version resolves which plugin artifact the running binary
// belongs to. The binary is always deployed at <plugin root>/bin/intermux-mcp
// (plugin cache, dev tree, launcher fallback build alike), so the manifest
// sitting next to it — not a compile-time constant — is the truth about what
// is actually running. That is what makes stale servers detectable after a
// publish wave: each process reports the version of its own artifact.
package version

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Fallback is reported when no plugin manifest is found next to the
// executable (e.g. a bare copy in ~/.local/bin). Overridable at link time:
//
//	go build -ldflags "-X github.com/mistakeknot/intermux/internal/version.Fallback=x.y.z"
var Fallback = "unknown"

// Info describes the running server artifact.
type Info struct {
	Version  string `json:"version"`            // plugin version, or Fallback
	Manifest string `json:"manifest,omitempty"` // manifest the version came from
	Binary   string `json:"binary,omitempty"`   // resolved executable path
}

// Resolve reads the version from the .claude-plugin/plugin.json one
// directory above the executable's bin/ dir.
func Resolve() Info {
	exe, err := os.Executable()
	if err != nil {
		return Info{Version: Fallback}
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return resolveFrom(exe)
}

// resolveFrom is the path-injectable core of Resolve, split out for tests.
func resolveFrom(exe string) Info {
	info := Info{Version: Fallback, Binary: exe}
	manifest := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".claude-plugin", "plugin.json"))
	data, err := os.ReadFile(manifest)
	if err != nil {
		return info
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &m); err != nil || m.Version == "" {
		return info
	}
	info.Version = m.Version
	info.Manifest = manifest
	return info
}
