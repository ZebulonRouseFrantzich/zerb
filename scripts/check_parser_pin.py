#!/usr/bin/env python3
"""Read-only PC-042 parser source/selection check; not host qualification.

The supplied Go executable must already be verified by the calling development
gate. This routine checker rejects static symlinks, but does not promise a
filesystem sandbox against concurrent hostile replacement. It never updates pins.
"""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import stat
import subprocess
import sys


MODULE = "github.com/pelletier/go-toml/v2"
VERSION = "v2.4.3-zerb.1"
PATCHED_MODULE_SUM = "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc="
UPSTREAM_VERSION = "v2.4.3"
UPSTREAM_COMMIT = "071a36c2a57244f2e70369bfc69889fda2a1f60f"
UPSTREAM_SUM = "h1:GTRvJQutkOSftxIFD5xw9aepkYNuPWmVJpffdDPYVpY="
GO_MOD_SUM = "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY="
ARCHIVE_SHA256 = "bc0de16d4942df8503be13bde0447e0b7ce1694f3bb1b840f485ce839f8974cf"
PATCH_FILE = "go-toml-decoder-reset.patch"
PATCH_SHA256 = "dba7c80bcbce36ee3ab0d681647528ed57426332f31c7c9595312285c48830b0"
PATCHED_SHA256 = "9203be82b3553d65b2eeecdb5b14b4e07729176792000d1c2359ff6b7a357c78"
ADDED_TESTS = ["decoder_pool_release_test.go"]
MANIFEST_FILE = "third_party/go-toml.provenance.json"
FORK_DIR = "third_party/go-toml"
HEX = re.compile(r"[0-9a-f]{64}\Z")


class PinError(Exception):
    """A fixed, metadata-free development diagnostic."""


class Arguments(argparse.ArgumentParser):
    def error(self, _message):
        self.exit(2, "parser pin: invalid arguments\n")


def require(condition, message):
    if not condition:
        raise PinError(message)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def hash1(files, prefix=""):
    """Go dirhash.Hash1, given relative names and their file SHA256 hex."""
    summary = hashlib.sha256()
    for name in sorted(files):
        summary.update((files[name] + "  " + prefix + name + "\n").encode("utf-8"))
    return "h1:" + base64.b64encode(summary.digest()).decode("ascii")


def safe_name(name):
    return (isinstance(name, str) and bool(name) and "\\" not in name
            and not any(ord(char) < 32 or ord(char) == 127 for char in name)
            and not name.startswith("/")
            and all(part not in ("", ".", "..") for part in name.split("/")))


def no_links(path, directory=False):
    """Check every existing ancestor, without resolving a link to its target."""
    require(".." not in Path(path).parts, "noncanonical parser pin path")
    path = Path(os.path.abspath(path))
    current = Path(path.anchor)
    for part in path.parts[1:]:
        current /= part
        mode = current.lstat().st_mode
        require(not stat.S_ISLNK(mode), "symlink in parser pin paths")
        if current != path:
            require(stat.S_ISDIR(mode), "invalid parser pin ancestor")
    mode = path.lstat().st_mode
    require(stat.S_ISDIR(mode) if directory else stat.S_ISREG(mode), "invalid parser pin file type")
    return path


def read_regular(path):
    return no_links(path).read_bytes()


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON key")
        result[key] = value
    return result


def read_json(data):
    try:
        return json.loads(data, object_pairs_hook=unique_object,
                          parse_constant=lambda _: (_ for _ in ()).throw(PinError("invalid JSON constant")))
    except (ValueError, UnicodeError, RecursionError):
        raise PinError("invalid pin JSON") from None


def exact_keys(value, keys):
    require(isinstance(value, dict) and set(value) == set(keys), "invalid provenance schema")


def manifest(root):
    value = read_json(read_regular(root / MANIFEST_FILE))
    exact_keys(value, ("schema", "module", "version", "module_sum", "go_mod_sum", "upstream", "patch", "added_tests"))
    require(type(value["schema"]) is int and value["schema"] == 1
            and value["module"] == MODULE and value["version"] == VERSION
            and value["module_sum"] == PATCHED_MODULE_SUM
            and value["go_mod_sum"] == GO_MOD_SUM and value["added_tests"] == ADDED_TESTS,
            "unexpected parser pin identity")
    upstream = value["upstream"]
    exact_keys(upstream, ("version", "origin_commit", "module_sum", "go_mod_sum", "archive_sha256", "files"))
    require(upstream["version"] == UPSTREAM_VERSION and upstream["origin_commit"] == UPSTREAM_COMMIT
            and upstream["module_sum"] == UPSTREAM_SUM and upstream["go_mod_sum"] == GO_MOD_SUM
            and upstream["archive_sha256"] == ARCHIVE_SHA256, "unexpected upstream identity")
    files = upstream["files"]
    require(isinstance(files, dict) and len(files) == 113
            and all(safe_name(name) and isinstance(digest, str) and HEX.fullmatch(digest)
                    for name, digest in files.items()), "invalid upstream file inventory")
    require(hash1(files, MODULE + "@" + UPSTREAM_VERSION + "/") == UPSTREAM_SUM,
            "upstream file inventory checksum mismatch")
    require("go.mod" in files and "unmarshaler.go" in files
            and not set(files).intersection(ADDED_TESTS), "invalid upstream file set")
    require(hash1({"go.mod": files["go.mod"]}) == GO_MOD_SUM, "upstream go.mod checksum mismatch")
    exact_keys(value["patch"], ("file", "sha256"))
    require(value["patch"] == {"file": PATCH_FILE, "sha256": PATCH_SHA256}, "unexpected parser patch identity")
    require(sha256(read_regular(root / "third_party" / PATCH_FILE)) == PATCH_SHA256,
            "parser patch checksum mismatch")
    return value


def fork_files(root):
    directory = no_links(root / FORK_DIR, directory=True)
    files = {}
    directories = set()

    def visit(path):
        with os.scandir(path) as entries:
            for entry in entries:
                require(not entry.is_symlink(), "symlink in parser fork")
                if entry.is_dir(follow_symlinks=False):
                    directories.add(Path(entry.path).relative_to(directory).as_posix())
                    visit(Path(entry.path))
                else:
                    require(entry.is_file(follow_symlinks=False), "nonregular parser fork entry")
                    name = Path(entry.path).relative_to(directory).as_posix()
                    require(safe_name(name), "invalid parser fork path")
                    files[name] = sha256(read_regular(entry.path))

    visit(directory)
    parents = set()
    for name in files:
        parent = Path(name).parent
        while parent != Path("."):
            parents.add(parent.as_posix())
            parent = parent.parent
    require(directories == parents, "extra empty parser fork directory")
    return files


def mod_lines(data):
    """Read Go-mod line declarations, preserving quoted // and ignoring comments."""
    for line in data.decode("utf-8").splitlines():
        quoted, escaped = False, False
        end = len(line)
        for index, char in enumerate(line):
            if escaped:
                escaped = False
            elif quoted and char == "\\":
                escaped = True
            elif char == '"':
                quoted = not quoted
            elif not quoted and line[index:index + 2] == "//":
                end = index
                break
        require(not quoted and not escaped, "invalid Go module declaration")
        try:
            yield shlex.split(line[:end], comments=False, posix=True)
        except ValueError:
            raise PinError("invalid Go module declaration") from None


def root_pin(root):
    requirements, replacements = [], []
    block = None
    for words in mod_lines(read_regular(root / "go.mod")):
        if not words:
            continue
        if len(words) == 2 and words[1] == "(":
            require(block is None, "invalid Go module block")
            block = words[0]
            continue
        if words == [")"]:
            require(block is not None, "invalid Go module block")
            block = None
            continue
        if block:
            kind, entry = block, words
        else:
            kind, entry = words[0], words[1:]
        if MODULE not in entry:
            continue
        if kind == "require":
            requirements.append(entry)
        elif kind == "replace":
            replacements.append(entry)
        else:
            raise PinError("unexpected Go TOML module declaration")
    require(block is None and requirements == [[MODULE, VERSION]]
            and replacements == [[MODULE, VERSION, "=>", "./" + FORK_DIR]], "root parser requirement/replacement mismatch")


def effective_selection(root, go):
    require(Path(go).is_absolute(), "Go executable must be absolute")
    executable = no_links(go)
    require(os.access(executable, os.X_OK), "Go executable is not executable")
    environment = os.environ.copy()
    environment.update({"GOFLAGS": "-mod=readonly -buildvcs=false", "GOPROXY": "off", "GOSUMDB": "off",
                        "GONOPROXY": "none", "GOPRIVATE": "", "GONOSUMDB": "none",
                        "GOVCS": "*:off", "GOAUTH": "off", "GOTOOLCHAIN": "local",
                        "GOWORK": "off", "GOENV": "off"})
    try:
        result = subprocess.run([str(executable), "list", "-m", "-json", MODULE], cwd=root,
                                env=environment, capture_output=True, check=False, timeout=60)
    except (OSError, subprocess.TimeoutExpired):
        raise PinError("Go module selection check failed") from None
    require(result.returncode == 0, "Go module selection check failed")
    selected = read_json(result.stdout)
    require(isinstance(selected, dict) and selected.get("Path") == MODULE
            and selected.get("Version") == VERSION, "effective parser selection mismatch")
    replacement = selected.get("Replace")
    require(isinstance(replacement, dict) and replacement.get("Path") == "./" + FORK_DIR
            and "Version" not in replacement, "effective parser replacement mismatch")
    expected = root / FORK_DIR
    require(isinstance(replacement.get("Dir"), str) and Path(replacement["Dir"]).is_absolute()
            and no_links(replacement["Dir"], directory=True) == expected
            and isinstance(selected.get("Dir"), str) and Path(selected["Dir"]).is_absolute()
            and no_links(selected["Dir"], directory=True) == expected, "effective parser directory mismatch")


def check(root, go):
    root = no_links(root, directory=True)
    value = manifest(root)
    files = fork_files(root)
    upstream = value["upstream"]["files"]
    require(set(files) == set(upstream).union(ADDED_TESTS), "parser fork file set mismatch")
    require(all(files[name] == (PATCHED_SHA256 if name == "unmarshaler.go" else digest)
                for name, digest in upstream.items()), "parser source checksum mismatch")
    require(hash1(files, MODULE + "@" + VERSION + "/") == value["module_sum"], "parser module checksum mismatch")
    require(hash1({"go.mod": files["go.mod"]}) == GO_MOD_SUM, "parser go.mod checksum mismatch")
    modules = [words for words in mod_lines(read_regular(root / FORK_DIR / "go.mod")) if words and words[0] == "module"]
    require(modules == [["module", MODULE]], "parser module path mismatch")
    root_pin(root)
    effective_selection(root, go)


def main(argv=None):
    arguments = Arguments(prog="check_parser_pin", description=__doc__)
    arguments.add_argument("--go", required=True, help="absolute Go executable already verified by the caller")
    arguments.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1], help="repository root")
    args = arguments.parse_args(argv)
    try:
        check(args.root, args.go)
    except (PinError, OSError, UnicodeError):
        # Never expose authored manifest/path/module data or child output.
        print("parser pin: verification failed", file=sys.stderr)
        return 1
    print("parser pin: source integrity and effective local module selection verified; not host qualification")
    return 0


if __name__ == "__main__":
    sys.exit(main())
