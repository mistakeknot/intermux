#!/usr/bin/env python3
"""Tests for fleet-version-skew.py.

Run: python3 scripts/test_fleet_version_skew.py

Two things here are deliberate and worth keeping:

  * The Linux resolution path is exercised ON macOS, through --proc-root. A
    check that only runs where you happen to be standing is how the other half
    of a cross-platform branch ships broken — which is the exact bug this
    repo already fixed once, in the watcher's /proc-only getCWD.

  * The end-to-end cases drive the real script as a subprocess against a stub
    `ps`, rather than re-implementing its logic in the test. A test that copies
    the branch certifies the copy; that failure has already happened in this
    estate (rig-file-drift-bead.sh passed a 4-case suite while filing duplicate
    beads).
"""
from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
SCRIPT = HERE / "fleet-version-skew.py"

# The script's filename is hyphenated because it is a CLI, so it cannot be
# imported by name.
_spec = importlib.util.spec_from_file_location("fleet_version_skew", SCRIPT)
if _spec is None or _spec.loader is None:
    raise SystemExit(f"cannot load {SCRIPT}")
fvs = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(fvs)


def make_plugin_tree(root: Path, version: str, name: str = "intermux") -> Path:
    """Build <root>/bin/intermux-mcp with a sibling plugin manifest."""
    (root / "bin").mkdir(parents=True, exist_ok=True)
    (root / ".claude-plugin").mkdir(parents=True, exist_ok=True)
    (root / ".claude-plugin" / "plugin.json").write_text(
        json.dumps({"name": name, "version": version})
    )
    exe = root / "bin" / "intermux-mcp"
    exe.write_text("#!/bin/sh\nexit 0\n")
    exe.chmod(0o755)
    return exe


class ParseEtime(unittest.TestCase):
    def test_formats(self):
        for etime, want in [
            ("05:03", 303),
            ("01:02:03", 3723),
            ("2-03:04:05", 183845),
            ("17-18:10:19", 1534219),
            ("  00:01  ", 1),
        ]:
            self.assertEqual(fvs.parse_etime(etime), want, etime)

    def test_unparseable_is_negative_not_zero(self):
        # -1 rather than 0: a zero would render as "0m" and read as a server
        # that just started, which is the opposite of unknown.
        for bad in ["", "garbage", "1:2:3:4"]:
            self.assertEqual(fvs.parse_etime(bad), -1, bad)

    def test_human_uptime(self):
        self.assertEqual(fvs.human_uptime(-1), "?")
        self.assertEqual(fvs.human_uptime(90), "1m")
        self.assertEqual(fvs.human_uptime(3723), "1h02m")
        self.assertEqual(fvs.human_uptime(1534219), "17d18h")


class ResolveExe(unittest.TestCase):
    def test_absolute_comm_is_the_answer(self):
        # macOS: ps already handed us the path.
        self.assertEqual(
            fvs.resolve_exe(123, "/opt/x/bin/intermux-mcp", Path("/nonexistent")),
            "/opt/x/bin/intermux-mcp",
        )

    def test_proc_exe_symlink(self):
        # Linux path, exercised on whatever platform runs this suite.
        with tempfile.TemporaryDirectory() as td:
            proc = Path(td) / "proc"
            (proc / "4242").mkdir(parents=True)
            target = Path(td) / "cache" / "0.1.9" / "bin" / "intermux-mcp"
            target.parent.mkdir(parents=True)
            target.write_text("")
            os.symlink(target, proc / "4242" / "exe")
            self.assertEqual(fvs.resolve_exe(4242, "intermux-mcp", proc), str(target))

    def test_unresolvable_returns_empty(self):
        with tempfile.TemporaryDirectory() as td:
            self.assertEqual(fvs.resolve_exe(999999, "intermux-mcp", Path(td)), "")


class ManifestFor(unittest.TestCase):
    def test_reads_sibling_manifest(self):
        with tempfile.TemporaryDirectory() as td:
            exe = make_plugin_tree(Path(td) / "0.1.12", "0.1.12")
            version, name, manifest, source = fvs.manifest_for(str(exe))
            self.assertEqual((version, name, source), ("0.1.12", "intermux", "manifest"))
            self.assertTrue(manifest.endswith("plugin.json"))

    def test_pruned_artifact_infers_from_path_and_says_so(self):
        # The 2026-08-11 case: publish pruned the cache dir under a running
        # server. The version must still be reported, but never as a reading.
        gone = "/nope/cache/interagency-marketplace/intermux/0.1.9/bin/intermux-mcp"
        version, name, manifest, source = fvs.manifest_for(gone)
        self.assertEqual(version, "0.1.9")
        self.assertEqual(name, "intermux")
        self.assertEqual(source, "path-inferred")
        self.assertEqual(manifest, "", "no manifest path may be claimed when none was read")

    def test_pruned_without_version_in_path(self):
        _, _, _, source = fvs.manifest_for("/nope/somewhere/bin/intermux-mcp")
        self.assertEqual(source, "pruned")

    def test_present_binary_with_unreadable_manifest_is_not_inferred(self):
        # The binary exists but its manifest does not: that is genuinely
        # undetermined, and must NOT borrow the pruned-artifact inference.
        with tempfile.TemporaryDirectory() as td:
            exe = Path(td) / "0.1.9" / "bin" / "intermux-mcp"
            exe.parent.mkdir(parents=True)
            exe.write_text("")
            self.assertEqual(fvs.manifest_for(str(exe)), ("", "", "", ""))


class Classify(unittest.TestCase):
    def test_statuses(self):
        self.assertEqual(fvs.classify("0.1.12", "0.1.12", "manifest"), "current")
        self.assertEqual(fvs.classify("0.1.11", "0.1.12", "manifest"), "stale")
        self.assertEqual(fvs.classify("0.1.13", "0.1.12", "manifest"), "ahead")
        self.assertEqual(fvs.classify("", "0.1.12", ""), "undetermined")
        self.assertEqual(fvs.classify("0.1.12", "", ""), "undetermined")

    def test_orphaned_beats_version_equality(self):
        # Even if the inferred version matches, a deleted artifact is not the
        # published one — the published one still exists on disk.
        self.assertEqual(fvs.classify("0.1.12", "0.1.12", "path-inferred"), "orphaned")
        self.assertEqual(fvs.classify("", "0.1.12", "pruned"), "orphaned")


class PublishedVersion(unittest.TestCase):
    def test_reads_marketplace_entry(self):
        with tempfile.TemporaryDirectory() as td:
            mp = Path(td) / "marketplace.json"
            mp.write_text(json.dumps({"plugins": [
                {"name": "other", "version": "9.9.9"},
                {"name": "intermux", "version": "0.1.12"},
            ]}))
            self.assertEqual(fvs.published_version(mp, "intermux"), "0.1.12")
            self.assertEqual(fvs.published_version(mp, "absent"), "")

    def test_missing_manifest_is_empty_not_a_crash(self):
        self.assertEqual(fvs.published_version(Path("/nope/marketplace.json"), "intermux"), "")


class EndToEnd(unittest.TestCase):
    """Drive the real script against a stub `ps`."""

    def run_script(self, ps_lines: list[str], extra: list[str] | None = None):
        with tempfile.TemporaryDirectory() as td:
            td = Path(td)
            stub_bin = td / "bin"
            stub_bin.mkdir()
            body = "\\n".join(ps_lines)
            (stub_bin / "ps").write_text(f'#!/bin/sh\nprintf "{body}\\n"\n')
            (stub_bin / "ps").chmod(0o755)
            env = dict(os.environ, PATH=f"{stub_bin}:{os.environ['PATH']}")
            cmd = [sys.executable, str(SCRIPT), "--json", "--published", "0.1.12"]
            cmd += extra or []
            p = subprocess.run(cmd, capture_output=True, text=True, env=env, cwd=td)
            return p.returncode, json.loads(p.stdout or "{}")

    def test_no_processes_is_a_pass(self):
        # A machine with no servers has no stale servers. This must not be a
        # warning, or every quiet machine reports a finding forever.
        rc, out = self.run_script([])
        self.assertEqual(rc, 0)
        self.assertEqual(out["counts"], {"current": 0, "stale": 0, "orphaned": 0,
                                         "ahead": 0, "undetermined": 0})

    def test_mixed_fleet_exit_and_fields(self):
        with tempfile.TemporaryDirectory() as td:
            cur = make_plugin_tree(Path(td) / "0.1.12", "0.1.12")
            old = make_plugin_tree(Path(td) / "0.1.11", "0.1.11")
            rc, out = self.run_script([
                f"  101 01:00:00 {cur}",
                f"  202 2-03:00:00 {old}",
                "  303 17-18:00:00 /gone/intermux/0.1.9/bin/intermux-mcp",
                "  404 00:05:00 /usr/bin/some-other-daemon",  # must be ignored
            ])
        self.assertEqual(rc, 1, "a fleet with stale servers must exit 1")
        self.assertEqual(out["counts"]["current"], 1)
        self.assertEqual(out["counts"]["stale"], 1)
        self.assertEqual(out["counts"]["orphaned"], 1)
        pids = [s["pid"] for s in out["servers"]]
        self.assertNotIn(404, pids, "non-matching process names must not be swept in")
        # Sorted by uptime, longest first: the worst offender leads.
        self.assertEqual(pids[0], 303)
        orphan = next(s for s in out["servers"] if s["pid"] == 303)
        self.assertEqual(orphan["version"], "0.1.9")
        self.assertEqual(orphan["version_source"], "path-inferred")
        self.assertEqual(orphan["uptime"], "17d18h")

    def test_all_current_exits_zero(self):
        with tempfile.TemporaryDirectory() as td:
            cur = make_plugin_tree(Path(td) / "0.1.12", "0.1.12")
            rc, out = self.run_script([f"  101 01:00:00 {cur}"])
        self.assertEqual(rc, 0)
        self.assertEqual(out["counts"]["current"], 1)

    def test_strict_flags_undetermined(self):
        with tempfile.TemporaryDirectory() as td:
            exe = Path(td) / "0.1.9" / "bin" / "intermux-mcp"
            exe.parent.mkdir(parents=True)
            exe.write_text("")
            rc, out = self.run_script([f"  101 01:00:00 {exe}"], extra=["--strict"])
        self.assertEqual(rc, 2)
        self.assertEqual(out["counts"]["undetermined"], 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
