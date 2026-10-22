#!/usr/bin/env python3
"""PC-042 parser source/selection check; not host qualification.

The supplied Go executable must already be verified by the calling development
gate. This routine checker rejects static symlinks, but does not promise a
filesystem sandbox against concurrent hostile replacement. The default is
read-only. Explicit --prepare materializes an absent local fork offline; atomic
no-clobber publication requires Linux renameat2. It never updates pins.
"""

import argparse
import base64
import ctypes
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import stat
import subprocess
import sys
import tempfile
import zipfile


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
ARCHIVE_FILE = "third_party/go-toml-v2.4.3.zip"
REGRESSION_FILE = "third_party/go-toml-decoder_pool_release_test.go.txt"
LICENSE_FILE = "third_party/go-toml.LICENSE"
REGRESSION_SHA256 = "fb042d9ee8293827f25b6092fc7b9c33807991d7fd706439ffe4c9a9cf9f5efc"
ARCHIVE_BYTES = 1_025_455
MAX_ARCHIVE_FILE = 1 << 20
MAX_ARCHIVE_TOTAL = 2 << 20
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
    return tree_files(root / FORK_DIR)


def tree_files(directory):
    directory = no_links(directory, directory=True)
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


def archive_files(data, upstream):
    # Authenticate the compressed bytes before invoking the ZIP parser.
    require(len(data) == ARCHIVE_BYTES and sha256(data) == ARCHIVE_SHA256, "parser archive checksum mismatch")
    prefix = MODULE + "@" + UPSTREAM_VERSION + "/"
    files, total = {}, 0
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            require(len(archive.infolist()) == 113, "parser archive entry count mismatch")
            for entry in archive.infolist():
                require(entry.filename.startswith(prefix), "parser archive prefix mismatch")
                name = entry.filename[len(prefix):]
                mode = entry.external_attr >> 16
                require(safe_name(name) and name in upstream and name not in files
                        and not entry.is_dir() and stat.S_IFMT(mode) in (0, stat.S_IFREG)
                        and not entry.external_attr & 0x10
                        and not entry.flag_bits & 1
                        and entry.compress_type in (zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED)
                        and 0 <= entry.file_size <= MAX_ARCHIVE_FILE, "invalid parser archive entry")
                total += entry.file_size
                require(total <= MAX_ARCHIVE_TOTAL, "parser archive size limit")
                content = archive.read(entry)
                require(len(content) == entry.file_size and sha256(content) == upstream[name], "parser archive file checksum mismatch")
                files[name] = content
    except (zipfile.BadZipFile, RuntimeError, NotImplementedError):
        raise PinError("invalid parser archive") from None
    require(set(files) == set(upstream), "parser archive file set mismatch")
    return files


def packaged_files(root, value):
    archive = no_links(root / ARCHIVE_FILE)
    require(archive.stat().st_size == ARCHIVE_BYTES, "parser archive size mismatch")
    files = archive_files(archive.read_bytes(), value["upstream"]["files"])
    require(sha256(read_regular(root / LICENSE_FILE)) == value["upstream"]["files"]["LICENSE"], "parser license mismatch")
    regression = read_regular(root / REGRESSION_FILE)
    require(sha256(regression) == REGRESSION_SHA256, "parser regression mismatch")
    # These two literal replacements are the reviewed, checksum-bound patch.
    # No general patch interpreter or authored program is executed.
    old = b"// decoderPool recycles decoders (and their internal buffers: parser arena,\n// seen-tracker entries, scratch buffers) across calls to Unmarshal and\n// Decode.\n"
    new = b"// decoderPool reuses only decoder shells; released decoders retain no input\n// references or input-dependent backing storage.\n"
    body = files["unmarshaler.go"]
    require(body.count(old) == 1 and body.count(b"func putDecoder(d *decoder) {\n") == 1, "parser patch context mismatch")
    files["unmarshaler.go"] = body.replace(old, new, 1).replace(b"func putDecoder(d *decoder) {\n", b"func putDecoder(d *decoder) {\n\t*d = decoder{}\n", 1)
    require(sha256(files["unmarshaler.go"]) == PATCHED_SHA256, "parser patch result mismatch")
    files[ADDED_TESTS[0]] = regression
    require(hash1({name: sha256(data) for name, data in files.items()}, MODULE + "@" + VERSION + "/") == PATCHED_MODULE_SUM,
            "packaged parser checksum mismatch")
    return files


def publish_noreplace(source, destination):
    require(sys.platform == "linux", "atomic parser preparation requires Linux")
    library = ctypes.CDLL(None, use_errno=True)
    rename = getattr(library, "renameat2", None)
    require(rename is not None, "atomic parser preparation unavailable")
    rename.argtypes = (ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint)
    rename.restype = ctypes.c_int
    if rename(-100, os.fsencode(source), -100, os.fsencode(destination), 1) != 0:
        raise PinError("atomic parser publication failed") from OSError(ctypes.get_errno(), "renameat2")


def write_staged(directory, files):
    for name, data in files.items():
        path = directory
        for part in Path(name).parts[:-1]:
            path /= part
            path.mkdir(mode=0o700, exist_ok=True)
        descriptor = os.open(directory / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "wb") as output:
            output.write(data)


def prepare(root):
    root = no_links(root, directory=True)
    value = manifest(root)
    files = packaged_files(root, value)
    destination = root / FORK_DIR
    if os.path.lexists(destination):
        verify_fork(root, value)
        return
    temporary = Path(tempfile.mkdtemp(prefix=".go-toml-", dir=root / "third_party"))
    try:
        write_staged(temporary, files)
        require(hash1(tree_files(temporary), MODULE + "@" + VERSION + "/") == PATCHED_MODULE_SUM,
                "staged parser checksum mismatch")
        publish_noreplace(temporary, destination)
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)


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


def verify_fork(root, value):
    files = fork_files(root)
    upstream = value["upstream"]["files"]
    require(set(files) == set(upstream).union(ADDED_TESTS), "parser fork file set mismatch")
    require(all(files[name] == (PATCHED_SHA256 if name == "unmarshaler.go" else digest)
                for name, digest in upstream.items()), "parser source checksum mismatch")
    require(hash1(files, MODULE + "@" + VERSION + "/") == value["module_sum"], "parser module checksum mismatch")
    require(hash1({"go.mod": files["go.mod"]}) == GO_MOD_SUM, "parser go.mod checksum mismatch")
    modules = [words for words in mod_lines(read_regular(root / FORK_DIR / "go.mod")) if words and words[0] == "module"]
    require(modules == [["module", MODULE]], "parser module path mismatch")


def check(root, go):
    root = no_links(root, directory=True)
    value = manifest(root)
    packaged_files(root, value)
    verify_fork(root, value)
    root_pin(root)
    effective_selection(root, go)


def main(argv=None):
    arguments = Arguments(prog="check_parser_pin", description=__doc__)
    arguments.add_argument("--go", required=True, help="absolute Go executable already verified by the caller")
    arguments.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1], help="repository root")
    arguments.add_argument("--prepare", action="store_true", help="offline materialization of an absent fork (Linux only)")
    args = arguments.parse_args(argv)
    try:
        require(Path(args.go).is_absolute(), "Go executable must be absolute")
        if args.prepare:
            prepare(args.root)
        check(args.root, args.go)
    except (PinError, OSError, UnicodeError, KeyboardInterrupt):
        # Never expose authored manifest/path/module data or child output.
        print("parser pin: verification failed", file=sys.stderr)
        return 1
    print("parser pin: source integrity and effective local module selection verified; not host qualification")
    return 0


if __name__ == "__main__":
    sys.exit(main())
