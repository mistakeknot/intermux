"""Vendored copies of the structural-test helpers shared across Interverse
plugins, so this repo's test suite has no dependency outside itself.

Source: ~/projects/Sylveste/interverse/_shared/tests/structural/
  - conftest_base.py -> create_structural_fixtures
  - helpers.py       -> parse_frontmatter
  - test_base.py     -> StructuralTests

test_required_root_files below drops the PHILOSOPHY.md requirement present
in the shared original: intermux does not carry a PHILOSOPHY.md (removed as
an internal planning artifact), so requiring it would fail this repo's own
suite on every run.
"""

import json
import os

import pytest
import yaml
from pathlib import Path


def create_structural_fixtures(project_root_path: Path) -> dict:
    """Generate standard structural test fixtures for a plugin.

    Returns a dict of fixture functions to be injected into the
    calling module's namespace.
    """

    @pytest.fixture(scope="session")
    def project_root() -> Path:
        """Path to the repository root."""
        return project_root_path

    @pytest.fixture(scope="session")
    def skills_dir(project_root: Path) -> Path:
        return project_root / "skills"

    @pytest.fixture(scope="session")
    def commands_dir(project_root: Path) -> Path:
        return project_root / "commands"

    @pytest.fixture(scope="session")
    def agents_dir(project_root: Path) -> Path:
        return project_root / "agents"

    @pytest.fixture(scope="session")
    def scripts_dir(project_root: Path) -> Path:
        return project_root / "scripts"

    @pytest.fixture(scope="session")
    def plugin_json(project_root: Path) -> dict:
        """Parsed plugin.json."""
        with open(project_root / ".claude-plugin" / "plugin.json") as f:
            return json.load(f)

    return {
        "project_root": project_root,
        "plugin_json": plugin_json,
        "skills_dir": skills_dir,
        "commands_dir": commands_dir,
        "agents_dir": agents_dir,
        "scripts_dir": scripts_dir,
    }


def parse_frontmatter(path):
    """Parse YAML frontmatter from a markdown file.

    Returns (frontmatter_dict, body_text) or (None, full_text) if no frontmatter.
    """
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---"):
        return None, text
    parts = text.split("---", 2)
    if len(parts) < 3:
        return None, text
    fm = yaml.safe_load(parts[1])
    body = parts[2]
    return fm, body


class StructuralTests:
    """Common structural tests shared across all Interverse plugins."""

    def test_plugin_json_valid(self, project_root, plugin_json):
        """plugin.json is valid JSON with required fields."""
        for field in ("name", "version", "description", "author"):
            assert field in plugin_json, (
                f"plugin.json missing required field: {field}"
            )

    def test_plugin_json_skills_match_filesystem(self, project_root, plugin_json):
        """Every skill listed in plugin.json exists on disk."""
        for skill_path in plugin_json.get("skills", []):
            resolved = project_root / skill_path
            assert resolved.is_dir(), f"Skill dir not found: {skill_path}"
            assert (resolved / "SKILL.md").exists(), (
                f"Missing SKILL.md in {skill_path}"
            )

    def test_plugin_json_commands_match_filesystem(self, project_root, plugin_json):
        """Every command listed in plugin.json exists on disk."""
        for cmd_path in plugin_json.get("commands", []):
            resolved = project_root / cmd_path
            assert resolved.exists(), f"Command not found: {cmd_path}"

    def test_required_root_files(self, project_root):
        """All required root-level files exist."""
        required = ["CLAUDE.md", "LICENSE", ".gitignore"]
        for name in required:
            assert (project_root / name).exists(), (
                f"Missing required file: {name}"
            )

    def test_scripts_executable(self, project_root):
        """All shell scripts are executable."""
        scripts_dir = project_root / "scripts"
        if not scripts_dir.is_dir():
            return
        for script in scripts_dir.glob("*.sh"):
            if script.name.startswith("lib-"):
                continue  # sourced libraries, never executed directly
            assert os.access(script, os.X_OK), (
                f"Script not executable: {script.name}"
            )
