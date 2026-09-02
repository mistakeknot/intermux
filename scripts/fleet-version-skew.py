#!/usr/bin/env python3
"""Report running intermux-mcp servers whose version is not the published one.

Why this exists
---------------
An intermux-mcp server is started once per Claude Code session and lives as long
as that session does. A publish wave updates the plugin cache; it does NOT touch
a process already running out of an older cache directory. So the fleet drifts
silently, and the drift is invisible from every surface anyone would think to
check: `ic publish status` compares version NUMBERS in files, and the plugin
cache is a directory named for a version rather than hashed on content.

Measured on a dev machine 2026-08-11, four days after 0.1.9 shipped and one day after
0.1.12: 27 servers running, 23 of them still on 0.1.9 — two publishes behind,
the oldest up 17 days. Every one of those sessions was calling `list_agents` and
getting pre-fix answers while the repo, the marketplace and the cache all read
current.

How a server's version is determined
------------------------------------
pid -> executable path -> the .claude-plugin/plugin.json sitting beside it.

That is the same resolution the server itself performs at startup
(internal/version), so this script reports exactly what a live `server_info`
call would report, without talking to the server at all. Two rejected
alternatives, and why:

  * Parsing the version out of the cache path (.../intermux/0.1.9/bin/...).
    Cheap and wrong twice over: a dev-tree binary's path carries no version at
    all, and a hand-patched cache directory keeps the old version in its name
    while holding new code. The manifest is the artifact's own claim; the path
    is a label someone else wrote.

  * Asking each server over MCP stdio. These processes are attached to live
    Claude Code sessions — their stdin is that session's pipe. Writing a
    JSON-RPC frame into it would interleave with the client's own traffic and
    desynchronise the stream. A monitoring tool must not be able to break the
    thing it monitors, which is why the enumeration here is entirely external.

Exit codes: 0 all current (or none running), 1 skew found, 2 undetermined
(with --strict). Nothing running is a pass, not an abstention: a machine with no
servers has no stale servers.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

DEFAULT_MARKETPLACE = (
    Path.home() / ".claude/plugins/marketplaces/interagency-marketplace/.claude-plugin/marketplace.json"
)
DEFAULT_PROCESS = "intermux-mcp"
DEFAULT_PLUGIN = "intermux"

# A ps that hangs would wedge a health run at a step that has no business
# blocking. Every external call below is bounded.
PS_TIMEOUT = 20


def run(cmd: list[str], timeout: int = PS_TIMEOUT) -> str:
    try:
        out = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    except (subprocess.TimeoutExpired, FileNotFoundError, OSError):
        return ""
    return out.stdout if out.returncode == 0 else ""


def parse_etime(etime: str) -> int:
    """Convert ps etime ([[dd-]hh:]mm:ss) to seconds.

    Parsed rather than read from `ps -o etimes=`, which is Linux-only: on macOS
    that keyword does not exist and ps rejects the whole invocation, taking
    every other column with it.
    """
    etime = etime.strip()
    m = re.fullmatch(r"(?:(?:(\d+)-)?(\d+):)?(\d+):(\d+)", etime)
    if not m:
        return -1
    days, hours, mins, secs = (int(g) if g else 0 for g in m.groups())
    return days * 86400 + hours * 3600 + mins * 60 + secs


def human_uptime(seconds: int) -> str:
    if seconds < 0:
        return "?"
    d, rem = divmod(seconds, 86400)
    h, rem = divmod(rem, 3600)
    m = rem // 60
    if d:
        return f"{d}d{h:02d}h"
    if h:
        return f"{h}h{m:02d}m"
    return f"{m}m"


def list_processes(process_name: str) -> list[tuple[int, str, int]]:
    """Return (pid, comm, uptime_seconds) for every matching process.

    One ps pass for all three facts. Matching is on the BASENAME of comm, which
    is the one field that behaves the same on both platforms: macOS reports the
    full executable path there, Linux reports the bare name.
    """
    out = run(["ps", "-axo", "pid=,etime=,comm="])
    found = []
    for line in out.splitlines():
        parts = line.split(None, 2)
        if len(parts) < 3:
            continue
        pid_s, etime, comm = parts
        if os.path.basename(comm.strip()) != process_name:
            continue
        try:
            pid = int(pid_s)
        except ValueError:
            continue
        found.append((pid, comm.strip(), parse_etime(etime)))
    return found


def resolve_exe(pid: int, comm: str, proc_root: Path) -> str:
    """Resolve a pid's executable path.

    Branches on evidence rather than on platform: if ps already handed us an
    absolute path (macOS), that IS the answer. Otherwise try /proc/<pid>/exe
    (Linux), then lsof's txt descriptor as a last resort. Sniffing
    sys.platform instead would make the untaken branch untestable on the
    machine you are standing on — the same reason the watcher's getCWD fix
    keeps both paths live.
    """
    if comm.startswith("/"):
        return comm
    try:
        return os.readlink(proc_root / str(pid) / "exe")
    except OSError:
        pass
    if shutil.which("lsof"):
        out = run(["lsof", "-a", "-p", str(pid), "-d", "txt", "-Fn"])
        for line in out.splitlines():
            if line.startswith("n/"):
                return line[1:].strip()
    return ""


def manifest_for(exe: str) -> tuple[str, str, str, str]:
    """(version, plugin_name, manifest_path, source) for a running binary.

    The binary always sits at <plugin root>/bin/, so the manifest is one level
    up — true for the plugin cache, the dev tree, and the launcher's go-build
    fallback alike.

    `source` names WHERE the version came from, because the two answers are not
    equally trustworthy and collapsing them is how an inference gets recorded as
    a reading. "manifest" is the artifact's own claim. "path-inferred" is a guess
    from the directory name, used only when the artifact is gone.
    """
    if not exe:
        return "", "", "", ""
    manifest = Path(exe).parent.parent / ".claude-plugin" / "plugin.json"
    try:
        data = json.loads(manifest.read_text())
        version = str(data.get("version") or "")
        if version:
            return version, str(data.get("name") or ""), str(manifest), "manifest"
    except (OSError, ValueError):
        pass

    # The artifact was deleted while this process kept running it. `ic publish`
    # prunes superseded cache directories, and Unix keeps an executing inode
    # alive after its last link is gone — so the server runs on happily from a
    # binary that no longer exists at any path. Observed on a dev machine 2026-08-11:
    # 23 servers executing a 0.1.9 cache directory that publish had already
    # pruned, their manifests unreadable because the whole tree was gone.
    #
    # Here, and only here, the directory name is the last surviving evidence of
    # what is running. It is reported as an inference, never as a reading.
    if not Path(exe).exists():
        m = re.search(r"/([^/]+)/(\d+\.\d+\.\d+[^/]*)/bin/", exe)
        if m:
            return m.group(2), m.group(1), "", "path-inferred"
        return "", "", "", "pruned"
    return "", "", "", ""


def version_key(v: str):
    parts = re.findall(r"\d+", v)
    return tuple(int(p) for p in parts) if parts else None


def published_version(marketplace: Path, plugin: str) -> str:
    try:
        data = json.loads(marketplace.read_text())
    except (OSError, ValueError):
        return ""
    for entry in data.get("plugins") or []:
        if entry.get("name") == plugin:
            return str(entry.get("version") or "")
    return ""


def classify(version: str, published: str, source: str) -> str:
    # An artifact that no longer exists on disk cannot be the published one —
    # the published version's directory is right there and this is not it. So
    # this is a definite finding, not an abstention, however the version reads.
    # Kept as its own status because the remedy is the same as stale (restart)
    # but the diagnosis is not: nobody can inspect what these are still serving.
    if source in ("path-inferred", "pruned"):
        return "orphaned"
    if not version or not published:
        return "undetermined"
    if version == published:
        return "current"
    a, b = version_key(version), version_key(published)
    if a and b and a > b:
        # A dev-tree binary can legitimately lead the marketplace between a
        # merge and its publish. Naming it separately keeps "you have not
        # restarted" apart from "you have not published".
        return "ahead"
    return "stale"


def main() -> int:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("--marketplace", type=Path, default=DEFAULT_MARKETPLACE)
    ap.add_argument("--plugin", default=DEFAULT_PLUGIN)
    ap.add_argument("--process-name", default=DEFAULT_PROCESS)
    ap.add_argument("--published", default="", help="override the published version")
    ap.add_argument("--proc-root", type=Path, default=Path("/proc"),
                    help="procfs root (testing seam for the Linux resolution path)")
    ap.add_argument("--strict", action="store_true", help="treat undetermined as failure")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    published = args.published or published_version(args.marketplace, args.plugin)

    servers = []
    for pid, comm, uptime in sorted(list_processes(args.process_name), key=lambda p: -p[2]):
        exe = resolve_exe(pid, comm, args.proc_root)
        version, name, manifest, source = manifest_for(exe)
        servers.append({
            "name": name or args.plugin,
            "pid": pid,
            "version": version or "unknown",
            "version_source": source or "unresolved",
            "uptime": human_uptime(uptime),
            "uptime_s": uptime,
            "binary": exe or comm,
            "manifest": manifest,
            "status": classify(version, published, source),
        })

    counts = {k: sum(1 for s in servers if s["status"] == k)
              for k in ("current", "stale", "orphaned", "ahead", "undetermined")}
    needs_restart = counts["stale"] + counts["orphaned"]
    summary = (
        f"{len(servers)} running {args.process_name}: {counts['current']} current, "
        f"{counts['stale']} stale, {counts['orphaned']} orphaned, {counts['ahead']} ahead, "
        f"{counts['undetermined']} undetermined (published {published or '?'})"
    )

    if args.json:
        print(json.dumps({"published": published, "summary": summary,
                          "counts": counts, "servers": servers}, indent=2))
    else:
        print(summary + "\n")
        if servers:
            print(f"{'PID':>7}  {'VERSION':<9} {'UPTIME':>7}  {'STATUS':<11} {'FROM':<13} BINARY")
            print("-" * 108)
            for s in servers:
                print(f"{s['pid']:>7}  {s['version']:<9} {s['uptime']:>7}  "
                      f"{s['status']:<11} {s['version_source']:<13} {s['binary']}")
        if needs_restart:
            # SAY WHO CAN CLOSE THIS. A restart is the only fix and only a human
            # holds those sessions; a line that reads as an action item for a
            # scheduler is one that stays red forever and stops being read.
            print(f"\n  Closed by RESTARTING those sessions — a publish cannot reach a "
                  f"process that is already running. Each pid above is one Claude Code "
                  f"session still serving {args.plugin} code from an older artifact.")
        if counts["orphaned"]:
            print("\n  ORPHANED = the cache directory was pruned by a later publish while "
                  "the process kept executing the deleted binary. Their versions are "
                  "inferred from the path (FROM=path-inferred), the only evidence left "
                  "once the manifest is gone.")
        if counts["undetermined"]:
            print("\n  Undetermined = the manifest beside that binary could not be read, "
                  "though the binary is still there; the process is running something "
                  "this script cannot name.")

    if needs_restart:
        return 1
    if args.strict and counts["undetermined"]:
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
