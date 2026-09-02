"""Shared fixtures for structural tests."""

from pathlib import Path

from _vendored import create_structural_fixtures

PROJECT_ROOT = Path(__file__).resolve().parents[2]
fixtures = create_structural_fixtures(PROJECT_ROOT)

# Register fixtures in this module's namespace so pytest discovers them
project_root = fixtures["project_root"]
plugin_json = fixtures["plugin_json"]
skills_dir = fixtures["skills_dir"]
commands_dir = fixtures["commands_dir"]
agents_dir = fixtures["agents_dir"]
scripts_dir = fixtures["scripts_dir"]
