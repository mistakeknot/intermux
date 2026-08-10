package version

import (
	"os"
	"path/filepath"
	"testing"
)

func layout(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if manifest != "" {
		if err := os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".claude-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, "bin", "intermux-mcp")
}

func TestResolveFromManifest(t *testing.T) {
	exe := layout(t, `{"name":"intermux","version":"9.9.9"}`)
	info := resolveFrom(exe)
	if info.Version != "9.9.9" {
		t.Fatalf("Version = %q, want 9.9.9", info.Version)
	}
	if info.Manifest == "" || info.Binary != exe {
		t.Fatalf("expected manifest and binary paths, got %+v", info)
	}
}

func TestResolveFallbackWhenNoManifest(t *testing.T) {
	exe := layout(t, "")
	info := resolveFrom(exe)
	if info.Version != Fallback {
		t.Fatalf("Version = %q, want fallback %q", info.Version, Fallback)
	}
	if info.Manifest != "" {
		t.Fatalf("Manifest should be empty when unresolved, got %q", info.Manifest)
	}
}

func TestResolveFallbackOnMalformedOrEmptyVersion(t *testing.T) {
	for name, manifest := range map[string]string{
		"malformed": `{not json`,
		"empty":     `{"name":"intermux","version":""}`,
	} {
		info := resolveFrom(layout(t, manifest))
		if info.Version != Fallback {
			t.Fatalf("%s: Version = %q, want fallback", name, info.Version)
		}
	}
}
