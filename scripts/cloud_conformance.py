#!/usr/bin/env python3
"""Local cloud evidence only; never qualifies the frozen host (VAL-001/002/003).

No dependency installation, privileged probes, external evidence import, or host
proof invocation is performed. Invoke from any directory after activating the
existing development environment. The output must be a new file outside the repo.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import pwd
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True

SCHEMA = "zerb-cloud-conformance-v1"
CA_PATH = "internal/proof/adapters/testdata/reviewed-ca-bundle.pem"
CA_SIZE = 472033
CA_SHA256 = "3ba2000fe3d55cb6c41323fabbb105743e2c9422151eea213311538b07b23f5a"
REQUIRED_CHECKS = (
    "proof-race", "vet", "lint", "modules", "release-build", "python-boundaries",
    "foundation-imports", "foundation-race",
    "parser-pin", "parser-upstream-race",
)
REQUIRED_TOOLS = ("git", "go", "golangci-lint", "python3", "cc")
TOOL_ARGS = {
    "git": ["--version"], "go": ["version"],
    "golangci-lint": ["version"], "python3": ["--version"],
    "cc": ["--version"], "bash": ["--version"], "mise": ["--version"],
    "chezmoi": ["--version"], "coreutils": ["--version"],
}
VERSION_PATTERNS = {
    "git": r"git version (\d+\.\d+\.\d+)",
    "go": r"go version (go\d+\.\d+(?:\.\d+)?)",
    "golangci-lint": r"(?:version |^)(v?\d+\.\d+\.\d+)",
    "python3": r"Python (\d+\.\d+\.\d+)",
    "cc": r"(?:\b)(\d+\.\d+\.\d+)",
    "bash": r"version (\d+\.\d+\.\d+)",
    "mise": r"(?:^|\s)(\d{4}\.\d+\.\d+)",
    "chezmoi": r"(?:version )?(v?\d+\.\d+\.\d+(?:-zerb\.\d+)?)",
    "coreutils": r"(?:coreutils\) |coreutils )?(\d+\.\d+)",
}
CANDIDATE_VERSIONS = {
    "git": "2.55.0", "bash": "5.3.15", "mise": "2026.7.17",
    "chezmoi": "v2.70.5-zerb.1", "coreutils": "9.11",
}
HOST_ROWS = (
    "exact-host-filesystem", "cgroup-resource-lifecycle", "namespace-isolation",
    "descriptor-supervisor", "git-acquisition", "mise-offline-install",
    "chezmoi-structural-mapping", "pty-shell", "installed-binary-acceptance",
)


class EvidenceError(Exception):
    """An evidence boundary failed; messages contain only fixed safe labels."""


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def inside(path, root):
    return path == root or root in path.parents


def metadata(st):
    return {"device": st.st_dev, "inode": st.st_ino,
            "mode": stat.S_IMODE(st.st_mode), "uid": st.st_uid,
            "gid": st.st_gid, "size": st.st_size,
            "mtime_ns": st.st_mtime_ns, "ctime_ns": st.st_ctime_ns,
            "type": "regular" if stat.S_ISREG(st.st_mode) else
            "directory" if stat.S_ISDIR(st.st_mode) else
            "character" if stat.S_ISCHR(st.st_mode) else "other"}


def no_symlink_components(path):
    path = Path(os.path.abspath(path))
    for parent in reversed((path, *path.parents)):
        if stat.S_ISLNK(parent.lstat().st_mode):
            raise EvidenceError("symlink-component")
    return path


def file_identity(path, limit=256 * 1024 * 1024, strict_path=False):
    """Hash one no-follow descriptor and verify path and descriptor after read."""
    original = Path(path)
    canonical = no_symlink_components(original) if strict_path else original.resolve(strict=True)
    fd = os.open(canonical, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size > limit:
            raise EvidenceError("invalid-file-type-or-size")
        digest = hashlib.sha256()
        count = 0
        while True:
            data = os.read(fd, 1024 * 1024)
            if not data:
                break
            count += len(data)
            if count > limit:
                raise EvidenceError("file-size-limit")
            digest.update(data)
        after = os.fstat(fd)
        if metadata(before) != metadata(after) or metadata(after) != metadata(canonical.lstat()):
            raise EvidenceError("file-identity-changed")
        if original.resolve(strict=True) != canonical or count != before.st_size:
            raise EvidenceError("file-path-changed")
        return {"path": str(canonical), **metadata(before), "sha256": digest.hexdigest()}
    finally:
        os.close(fd)


def ca_identity(repo):
    try:
        identity = file_identity(repo / CA_PATH, CA_SIZE, strict_path=True)
        if identity["size"] != CA_SIZE or identity["sha256"] != CA_SHA256:
            raise EvidenceError("ca-content-mismatch")
        return {"status": "passed", "identity": identity}
    except (OSError, EvidenceError):
        return {"status": "failed", "reason": "ca-missing-unsafe-or-not-exact"}


def protected_roots():
    roots = [Path(pwd.getpwuid(os.getuid()).pw_dir).resolve()]
    for key in ("HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME"):
        value = os.environ.get(key)
        if value and Path(value).is_absolute():
            roots.append(Path(value).resolve())
    return roots


def environment(temp, path, repo=None):
    """Construct child roots without retaining ambient credentials or config."""
    home = temp / "home"
    cache = temp / "cache"
    for directory in (home, cache, temp / "data", temp / "state", temp / "config", temp / "tmp"):
        directory.mkdir(mode=0o700, exist_ok=True)
    env = {
        "PATH": path, "HOME": str(home), "XDG_CACHE_HOME": str(cache),
        "XDG_DATA_HOME": str(temp / "data"), "XDG_STATE_HOME": str(temp / "state"),
        "XDG_CONFIG_HOME": str(temp / "config"), "TMPDIR": str(temp / "tmp"),
        "LANG": "C", "LC_ALL": "C", "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_CONFIG_GLOBAL": os.devnull, "GIT_TERMINAL_PROMPT": "0",
        "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off",
        "GOFLAGS": "-mod=readonly -buildvcs=false", "GOMAXPROCS": "4",
        "PYTHONDONTWRITEBYTECODE": "1", "ZERB_DEV": "1", "ZERB_TEST_MODE": "1",
        "ZERB_TEST_ROOT": str(temp / "proof-roots"),
    }
    # Existing caches are explicitly permitted build inputs, never HOME/config.
    protected = protected_roots()
    if repo is not None:
        protected.append(repo.resolve())
    for key in ("GOPATH", "GOCACHE"):
        value = os.environ.get(key)
        if value:
            candidate = Path(value)
            if not candidate.is_absolute() or not candidate.is_dir():
                raise EvidenceError("build-cache-path-invalid")
            canonical = candidate.resolve(strict=True)
            if any(inside(canonical, root) or inside(root, canonical) for root in protected):
                raise EvidenceError("build-cache-overlaps-protected-root")
            env[key] = str(canonical)
    env.setdefault("GOPATH", str(temp / "gopath"))
    env.setdefault("GOCACHE", str(temp / "go-cache"))
    return env


def capture(argv, repo, env, timeout=30):
    result = subprocess.run(argv, cwd=repo, env=env, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                            timeout=timeout, check=False)
    if result.returncode != 0 or len(result.stdout) > 32 * 1024 * 1024:
        raise EvidenceError("identity-command-failed")
    return result.stdout


def tool_inventory(repo, env):
    result = {}
    for name, args in TOOL_ARGS.items():
        identity = None
        try:
            found = sys.executable if name == "python3" else shutil.which(name, path=env["PATH"])
            if not found:
                raise EvidenceError("tool-missing")
            identity = file_identity(Path(found))
            output = capture([identity["path"], *args], repo, env).decode("utf-8", "replace")
            match = re.search(VERSION_PATTERNS[name], output)
            if not match:
                raise EvidenceError("tool-version-unknown")
            identity["version"] = match.group(1)
            if identity != {**file_identity(Path(found)), "version": match.group(1)}:
                raise EvidenceError("tool-changed-during-version")
            entry = {"status": "passed", "identity": identity}
        except (OSError, EvidenceError, subprocess.SubprocessError):
            entry = {"status": "failed", "reason": "tool-missing-unsafe-or-version-unknown"}
            if identity is not None:
                entry["identity"] = identity
        if name in CANDIDATE_VERSIONS:
            entry["candidate_version"] = CANDIDATE_VERSIONS[name]
            entry["candidate_match"] = {
                "status": "not-exercised" if entry["status"] == "passed" else "failed",
                "version_match": "passed" if entry.get("identity", {}).get("version") == CANDIDATE_VERSIONS[name]
                else "failed", "identity_qualification": "not-exercised",
                "provenance_and_frozen_digest_match": "not-exercised",
            }
        result[name] = entry
    return result


def source_identity(repo, git, env):
    commit = capture([git, "rev-parse", "--verify", "HEAD"], repo, env).decode().strip()
    if not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", commit):
        raise EvidenceError("source-commit-malformed")
    status_bytes = capture([git, "status", "--porcelain=v1", "-z", "--untracked-files=all",
                            "--ignore-submodules=all"], repo, env)
    index = capture([git, "ls-files", "--stage", "-z"], repo, env)
    gitlinks = {}
    for entry in index.split(b"\0"):
        if entry.startswith(b"160000 "):
            header, name = entry.split(b"\t", 1)
            gitlinks[name] = header.split()[1].decode("ascii")
    # Include ignored files too: an ignored Go/Python file can affect checks.
    names = capture([git, "ls-files", "--cached", "--others", "-z"], repo, env).split(b"\0")
    digest = hashlib.sha256()
    count = 0
    for raw in sorted(set(names) - {b""}):
        path = repo / os.fsdecode(raw)
        if not inside(Path(os.path.abspath(path)), repo):
            raise EvidenceError("source-path-outside-repository")
        digest.update(len(raw).to_bytes(8, "big") + raw)
        if raw in gitlinks:
            digest.update(b"gitlink-pointer-not-traversed:" + gitlinks[raw].encode())
            count += 1
            continue
        try:
            info = path.lstat()
        except FileNotFoundError:
            digest.update(b"missing")
            continue
        if stat.S_ISLNK(info.st_mode):
            # Go/Python can follow source links, so link text alone cannot bind
            # the bytes they would consume. Never read an external link target.
            raise EvidenceError("symlink-source-entry")
        elif stat.S_ISREG(info.st_mode):
            identity = file_identity(path, strict_path=True)
            binding = {key: identity[key] for key in ("sha256", "size", "mode", "device", "inode", "mtime_ns", "ctime_ns")}
        else:
            raise EvidenceError("source-nonregular-entry")
        digest.update(json.dumps(binding, sort_keys=True).encode())
        count += 1
    return {"status": "passed", "commit": commit, "dirty": bool(status_bytes),
            "status_sha256": sha256(status_bytes), "index_sha256": sha256(index),
            "tree_sha256": digest.hexdigest(), "file_count": count,
            "binding": "commit+index+status+all-tracked-and-untracked-including-ignored-file-identities; gitlinks-pointer-only"}


def mount_inventory():
    rows = []
    for line in Path("/proc/self/mountinfo").read_text().splitlines():
        left, right = line.split(" - ", 1)
        fields, tail = left.split(), right.split()
        mountpoint = re.sub(r"\\([0-7]{3})", lambda m: chr(int(m.group(1), 8)), fields[4])
        rows.append({"mount_id": int(fields[0]), "mountpoint": mountpoint, "filesystem": tail[0],
                     "mount_device": fields[2],
                     "read_only": "ro" in fields[5].split(",") or "ro" in tail[2].split(",")})
    if not rows:
        raise EvidenceError("mount-inventory-empty")
    return rows


def filesystem_identity(path, mounts):
    path = Path(path).resolve(strict=True)
    info = path.stat()
    device = str(os.major(info.st_dev)) + ":" + str(os.minor(info.st_dev))
    matches = [row for row in mounts if inside(path, Path(row["mountpoint"]))
               and row["mount_device"] == device]
    if not matches:
        raise EvidenceError("filesystem-unknown")
    row = max(enumerate(matches), key=lambda item: (len(item[1]["mountpoint"]), item[1].get("mount_id", item[0])))[1]
    return {"path": str(path), **metadata(info), **row}


def read_optional(path, limit=65536):
    try:
        with open(path, "rb") as stream:
            data = stream.read(limit + 1)
        return data.decode("ascii") if len(data) <= limit else None
    except (OSError, UnicodeError):
        return None


def host_inventory(repo, output_parent, temp):
    mounts = mount_inventory()
    uname = platform.uname()
    distro = {}
    os_release = read_optional("/etc/os-release")
    if os_release:
        for line in os_release.splitlines():
            if line.startswith(("ID=", "VERSION_ID=")):
                key, value = line.split("=", 1)
                distro[key] = value.strip('"')[:80]
    if not uname.system or not uname.release or not uname.machine or not distro.get("ID") or not distro.get("VERSION_ID"):
        raise EvidenceError("host-identity-incomplete")
    filesystems = {key: filesystem_identity(path, mounts) for key, path in
                   (("repository", repo), ("output_parent", output_parent), ("temporary", temp))}
    namespaces = {}
    for name in ("user", "mnt", "pid", "net", "cgroup", "ipc", "uts", "time"):
        try:
            link = os.readlink("/proc/self/ns/" + name)
            namespaces[name] = {"status": "not-exercised", "identity": link}
        except OSError:
            namespaces[name] = {"status": "not-exercised", "identity": None}
    cgroups = [row for row in mounts if row["filesystem"] == "cgroup2"]
    cgroup = {"status": "not-exercised", "mount_present": bool(cgroups),
              "mount_read_only": cgroups[0]["read_only"] if cgroups else None,
              "delegation": "not-exercised", "controllers": None,
              "subtree_control": None,
              "current_membership": read_optional("/proc/self/cgroup")}
    if cgroups:
        controllers = read_optional(Path(cgroups[0]["mountpoint"]) / "cgroup.controllers")
        cgroup["controllers"] = controllers.split() if controllers is not None else None
        subtree = read_optional(Path(cgroups[0]["mountpoint"]) / "cgroup.subtree_control")
        cgroup["subtree_control"] = subtree.split() if subtree is not None else None
    proc = next((row for row in mounts if row["mountpoint"] == "/proc"), None)
    try:
        ptmx = metadata(Path("/dev/ptmx").stat())
    except OSError:
        ptmx = None
    available_fs = read_optional("/proc/filesystems")
    btrfs = None if available_fs is None else any(line.split()[-1:] == ["btrfs"] for line in available_fs.splitlines())
    matches = {"distribution": distro == {"ID": "fedora", "VERSION_ID": "44"},
               "kernel": uname.release == "7.1.10-200.fc44.x86_64",
               "architecture": uname.machine == "x86_64",
               "repository_filesystem": filesystems["repository"]["filesystem"] == "btrfs"}
    return {"status": "passed", "kernel": {"system": uname.system, "release": uname.release,
            "version": uname.version, "machine": uname.machine}, "distribution": distro,
            "filesystems": filesystems, "candidate_host_match": {
                "status": "passed" if all(matches.values()) else "failed", "fields": matches,
                "qualification": "not-exercised"},
            "capabilities": {"cgroup_v2": cgroup, "namespaces": namespaces,
                "btrfs": {"status": "not-exercised", "kernel_registered": btrfs},
                "proc": {"status": "not-exercised", "mount_present": proc is not None,
                         "filesystem": proc["filesystem"] if proc else None},
                "ptmx": {"status": "not-exercised", "identity": ptmx}}}


def commands(tools, repo, temp):
    go = tools["go"]["identity"]["path"]
    return (
        ("proof-race", [go, "test", "-race", "-count=1", "./internal/proof/..."]),
        ("vet", [go, "vet", "./..."]),
        ("lint", [tools["golangci-lint"]["identity"]["path"], "run"]),
        ("modules", [go, "mod", "verify"]),
        ("release-build", [go, "build", "-trimpath", "-o", str(temp / "zerb"), "./cmd/zerb"]),
        ("python-boundaries", [tools["python3"]["identity"]["path"], "-m", "unittest", "discover",
                               "-s", "scripts", "-p", "test_*.py"]),
        ("foundation-imports", [go, "run", "./tools/importcheck"]),
        ("foundation-race", [go, "test", "-race", "-count=1", "./internal/paths", "./internal/safefs", "./internal/profile", "./internal/receipt", "./internal/toolchain", "./internal/state", "./internal/execenv", "./tools/importcheck"]),
        ("parser-pin", [tools["python3"]["identity"]["path"], "scripts/check_parser_pin.py", "--go", go]),
        ("parser-upstream-race", [go, "-C", "third_party/go-toml", "test", "-race", "-count=1", "./..."]),
    )


def run_check(name, argv, repo, env, temp):
    start = time.monotonic()
    # Private, disposable logs may contain raw tool diagnostics; the JSON never does.
    with tempfile.TemporaryFile(dir=temp) as log:
        try:
            result = subprocess.run(argv, cwd=repo, env=env, stdin=subprocess.DEVNULL,
                                    stdout=log, stderr=subprocess.STDOUT, timeout=1800, check=False)
            code = result.returncode
            reason = "exit-zero" if code == 0 else "command-nonzero"
        except (OSError, subprocess.SubprocessError):
            code, reason = None, "command-unavailable-or-timeout"
        log.seek(0, os.SEEK_END)
        size = log.tell()
    return {"name": name, "status": "passed" if code == 0 else "failed", "argv": argv,
            "exit_code": code, "reason": reason, "diagnostic_bytes": size,
            "elapsed_seconds": round(time.monotonic() - start, 3), "diagnostics": "omitted"}


def valid_identity(entry):
    if not isinstance(entry, dict) or entry.get("status") != "passed":
        return False
    identity = entry.get("identity")
    return (isinstance(identity, dict) and isinstance(identity.get("path"), str)
            and Path(identity["path"]).is_absolute()
            and identity.get("type") == "regular"
            and all(type(identity.get(key)) is int and identity[key] >= 0
                    for key in ("device", "inode", "size", "mode", "uid", "gid", "mtime_ns", "ctime_ns"))
            and bool(re.fullmatch(r"[0-9a-f]{64}", str(identity.get("sha256", "")))))


def nullable(value, kind):
    return value is None or type(value) is kind


def valid_capabilities(inventory):
    capabilities = inventory["capabilities"]
    if set(capabilities) != {"cgroup_v2", "namespaces", "btrfs", "proc", "ptmx"}:
        return False
    for key in ("cgroup_v2", "btrfs", "proc", "ptmx"):
        if capabilities[key]["status"] != "not-exercised":
            return False
    cgroup = capabilities["cgroup_v2"]
    if (type(cgroup["mount_present"]) is not bool or not nullable(cgroup["mount_read_only"], bool)
            or cgroup["delegation"] != "not-exercised" or not nullable(cgroup["current_membership"], str)):
        return False
    if cgroup["mount_present"] != (type(cgroup["mount_read_only"]) is bool):
        return False
    for key in ("controllers", "subtree_control"):
        tokens = cgroup[key]
        if tokens is not None and (type(tokens) is not list or
                any(not isinstance(token, str) or not re.fullmatch(r"[a-z0-9_]+", token) for token in tokens)
                or len(tokens) != len(set(tokens))):
            return False
    if not nullable(capabilities["btrfs"]["kernel_registered"], bool):
        return False
    proc = capabilities["proc"]
    if (type(proc["mount_present"]) is not bool or not nullable(proc["filesystem"], str)
            or proc["mount_present"] != bool(proc["filesystem"])):
        return False
    ptmx = capabilities["ptmx"]["identity"]
    if ptmx is not None:
        if (not isinstance(ptmx, dict) or ptmx.get("type") not in ("regular", "directory", "character", "other")
                or not all(type(ptmx.get(key)) is int and ptmx[key] >= 0 for key in
                           ("device", "inode", "mode", "uid", "gid", "size", "mtime_ns", "ctime_ns"))):
            return False
    namespaces = capabilities["namespaces"]
    if set(namespaces) != {"user", "mnt", "pid", "net", "cgroup", "ipc", "uts", "time"}:
        return False
    for name, row in namespaces.items():
        if row["status"] != "not-exercised" or (row["identity"] is not None and
                (not isinstance(row["identity"], str) or not re.fullmatch(name + r":\[\d+\]", row["identity"]))):
            return False
    match = inventory["candidate_host_match"]
    fields = match["fields"]
    expected_fields = {
        "distribution": inventory["distribution"] == {"ID": "fedora", "VERSION_ID": "44"},
        "kernel": inventory["kernel"]["release"] == "7.1.10-200.fc44.x86_64",
        "architecture": inventory["kernel"]["machine"] == "x86_64",
        "repository_filesystem": inventory["filesystems"]["repository"]["filesystem"] == "btrfs",
    }
    if (set(fields) != {"distribution", "kernel", "architecture", "repository_filesystem"}
            or any(type(value) is not bool for value in fields.values())
            or fields != expected_fields
            or match["status"] != ("passed" if all(fields.values()) else "failed")
            or match["qualification"] != "not-exercised"):
        return False
    return True


def gate_status(report):
    """Fail closed on omitted/malformed evidence; statuses are never imported."""
    try:
        if report["schema"] != SCHEMA or report["host_qualified"] is not False or report["release_supported"] is not False:
            return "failed"
        if report["inventory"].get("status") != "passed" or report["stability"].get("status") != "passed":
            return "failed"
        inventory = report["inventory"]
        if not all(inventory.get(key) for key in ("kernel", "distribution", "filesystems", "capabilities")):
            return "failed"
        if (not all(isinstance(inventory["kernel"].get(key), str) and inventory["kernel"][key]
                    for key in ("system", "release", "version", "machine"))
                or not all(isinstance(inventory["distribution"].get(key), str) and inventory["distribution"][key]
                           for key in ("ID", "VERSION_ID"))):
            return "failed"
        for key in ("repository", "output_parent", "temporary"):
            fs = inventory["filesystems"][key]
            if (not all(isinstance(fs.get(field), str) and fs[field] for field in
                        ("path", "mountpoint", "filesystem", "mount_device"))
                    or fs.get("type") != "directory"
                    or type(fs.get("read_only")) is not bool
                    or not all(type(fs.get(field)) is int and fs[field] >= 0 for field in ("device", "inode"))):
                return "failed"
        if not valid_capabilities(inventory):
            return "failed"
        for label in ("before", "after"):
            tools = report["tools"][label]
            if set(tools) != set(TOOL_ARGS) or any(row.get("status") not in ("passed", "failed") for row in tools.values()):
                return "failed"
            for tool in tools.values():
                if tool["status"] == "passed" and (not valid_identity(tool)
                        or not isinstance(tool["identity"].get("version"), str)
                        or not re.fullmatch(r"[A-Za-z0-9.+_-]{1,80}", tool["identity"]["version"])):
                    return "failed"
            for name, version in CANDIDATE_VERSIONS.items():
                candidate = tools[name]["candidate_match"]
                observed = tools[name].get("identity", {}).get("version")
                if (tools[name]["candidate_version"] != version
                        or candidate["status"] != ("not-exercised" if tools[name]["status"] == "passed" else "failed")
                        or candidate["version_match"] != ("passed" if observed == version else "failed")
                        or candidate["identity_qualification"] != "not-exercised"
                        or candidate["provenance_and_frozen_digest_match"] != "not-exercised"):
                    return "failed"
            source = report["source"][label]
            if (source.get("status") != "passed" or type(source.get("dirty")) is not bool
                    or not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", source.get("commit", ""))
                    or not all(re.fullmatch(r"[0-9a-f]{64}", source.get(key, ""))
                               for key in ("tree_sha256", "index_sha256", "status_sha256"))
                    or type(source.get("file_count")) is not int or source["file_count"] < 1):
                return "failed"
            ca = report["ca"][label]
            if not valid_identity(ca) or ca["identity"]["size"] != CA_SIZE or ca["identity"]["sha256"] != CA_SHA256:
                return "failed"
            if not all(valid_identity(report["tools"][label][name]) and
                       isinstance(report["tools"][label][name]["identity"].get("version"), str) and
                       re.fullmatch(r"[A-Za-z0-9.+_-]{1,80}", report["tools"][label][name]["identity"]["version"])
                       for name in REQUIRED_TOOLS):
                return "failed"
            if not re.fullmatch(r"go1\.24(?:\.\d+)?", report["tools"][label]["go"]["identity"]["version"]):
                return "failed"
        if any(report[key]["before"] != report[key]["after"] for key in ("source", "ca", "tools")):
            return "failed"
        host_rows = report["host_execution"]
        if set(host_rows) != set(HOST_ROWS) or any(row.get("status") != "not-exercised" for row in host_rows.values()):
            return "failed"
        checks = report["checks"]
        if len(checks) != len(REQUIRED_CHECKS) or {row["name"] for row in checks} != set(REQUIRED_CHECKS):
            return "failed"
        if report["inventory_only"] is True:
            return "not-exercised" if all(row["status"] == "not-exercised" for row in checks) else "failed"
        if report["inventory_only"] is not False:
            return "failed"
        return "passed" if all(row["status"] == "passed" and type(row.get("exit_code")) is int
                               and row["exit_code"] == 0 for row in checks) else "failed"
    except (KeyError, TypeError, AttributeError):
        return "failed"


def reserve_output(path, repo):
    raw = Path(os.path.abspath(path))
    parent = no_symlink_components(raw.parent)
    forbidden = [repo, *protected_roots()]
    if any(inside(raw, root) or inside(parent.resolve(), root) for root in forbidden):
        raise EvidenceError("output-must-be-outside-protected-roots")
    # Resolve no component through a symlink, including during the final create.
    directory = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in parent.parts[1:]:
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                            dir_fd=directory)
            os.close(directory)
            directory = child
        held_path = Path(os.readlink("/proc/self/fd/" + str(directory)))
        if any(inside(held_path, root) for root in forbidden):
            raise EvidenceError("output-parent-moved-into-protected-root")
        return os.open(raw.name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
                       0o600, dir_fd=directory)
    finally:
        os.close(directory)


def collect(repo, output_parent, temp, inventory_only):
    try:
        env = environment(temp, os.environ.get("PATH", os.defpath), repo)
    except (OSError, EvidenceError):
        env = None
    report = {"schema": SCHEMA, "status": "failed", "inventory_only": inventory_only,
              "host_qualified": False, "release_supported": False,
              "requirements": ["VAL-001", "VAL-002", "VAL-003"],
              "execution_policy": {"dependencies": "readonly-existing-only", "go_flags": "-mod=readonly -buildvcs=false",
                "buildvcs": False, "buildvcs_reason": "process-local linked-cloud-worktree metadata workaround",
                "network_dependency_resolution": False, "child_roots": "disposable",
                "logs": "private-disposable-outside-repository; raw diagnostics omitted"},
              "source": {}, "tools": {}, "ca": {}, "inventory": {"status": "failed"},
              "stability": {"status": "failed"},
              "checks": [{"name": name, "status": "not-exercised"} for name in REQUIRED_CHECKS],
              "host_execution": {name: {"status": "not-exercised"} for name in HOST_ROWS}}
    try:
        if env is None:
            raise EvidenceError("build-cache-environment-invalid")
        report["tools"]["before"] = tool_inventory(repo, env)
        report["ca"]["before"] = ca_identity(repo)
        git = report["tools"]["before"].get("git", {}).get("identity", {}).get("path")
        if git is None:
            raise EvidenceError("git-unavailable")
        report["source"]["before"] = source_identity(repo, git, env)
        report["inventory"] = host_inventory(repo, output_parent, temp)
        ready = (all(valid_identity(report["tools"]["before"].get(name)) for name in REQUIRED_TOOLS)
                 and report["ca"]["before"]["status"] == "passed")
        if ready:
            env["CC"] = report["tools"]["before"]["cc"]["identity"]["path"]
            report["execution_policy"]["cc"] = env["CC"]
            report["execution_policy"]["build_cache_paths"] = {key: env[key] for key in ("GOPATH", "GOCACHE")}
        if not inventory_only and ready:
            report["checks"] = [run_check(name, argv, repo, env, temp)
                                for name, argv in commands(report["tools"]["before"], repo, temp)]
        report["source"]["after"] = source_identity(repo, git, env)
        report["ca"]["after"] = ca_identity(repo)
        report["tools"]["after"] = tool_inventory(repo, env)
        report["stability"] = {"status": "passed" if all(report[key]["before"] == report[key]["after"]
                                for key in ("source", "ca", "tools")) else "failed"}
    except (OSError, ValueError, EvidenceError, subprocess.SubprocessError):
        report["failure"] = "critical-inventory-or-identity-unavailable"
    report["status"] = gate_status(report)
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, help="new JSON file outside repository")
    parser.add_argument("--inventory-only", action="store_true")
    args = parser.parse_args(argv)
    repo = Path(__file__).resolve().parent.parent
    try:
        temporary_base = no_symlink_components(Path("/tmp"))
        if any(inside(temporary_base, root) for root in [repo, *protected_roots()]):
            raise EvidenceError("temporary-root-overlaps-protected-root")
        fd = reserve_output(args.output, repo)
    except (OSError, EvidenceError):
        print("cloud conformance: output and temporary roots must be safe and outside repository and HOME/XDG roots", file=sys.stderr)
        return 1
    with os.fdopen(fd, "w", encoding="utf-8") as stream:
        # Explicit /tmp avoids an ambient TMPDIR pointing inside the repository.
        with tempfile.TemporaryDirectory(prefix="zerb-cloud-", dir="/tmp") as directory:
            report = collect(repo, Path(args.output).absolute().parent, Path(directory), args.inventory_only)
        json.dump(report, stream, indent=2, sort_keys=True)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    print("cloud conformance: " + report["status"])
    return 1 if report["status"] == "failed" else 0


if __name__ == "__main__":
    sys.exit(main())
