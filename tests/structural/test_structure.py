"""Tests for plugin structure."""

from _vendored import StructuralTests


class TestStructure(StructuralTests):
    """Structural tests -- inherits shared base, adds plugin-specific checks."""

    def test_plugin_name(self, plugin_json):
        assert plugin_json["name"] == "intermux"

    def test_scripts_count(self, project_root):
        """Expected number of scripts."""
        scripts_dir = project_root / "scripts"
        if not scripts_dir.is_dir():
            assert False, "Expected scripts/ directory"
            return
        scripts = list(scripts_dir.glob("*.sh"))
        assert len(scripts) == 2, (
            f"Expected 2 scripts, found {len(scripts)}: {[s.name for s in scripts]}"
        )
