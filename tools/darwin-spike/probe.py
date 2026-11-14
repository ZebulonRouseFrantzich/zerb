#!/usr/bin/env python3
"""Disposable Darwin primitive diagnostics, not a product backend or support proof."""
import ctypes
import errno
import fcntl
import json
import os
from pathlib import Path
import platform
import signal
import subprocess
import sys
import tempfile


def require(condition, label):
    if not condition:
        raise RuntimeError(label)


def run(root):
    # ARCH-009/010/017/020; VAL-020/025: only bounded diagnostic subsets.
    results = {}
    env = {"PATH": "/usr/bin:/bin", "HOME": str(root), "TMPDIR": str(root),
           "XDG_DATA_HOME": str(root / "data"), "XDG_CACHE_HOME": str(root / "cache"),
           "XDG_STATE_HOME": str(root / "state")}
    libc = ctypes.CDLL(None, use_errno=True)
    rename = libc.renameatx_np
    rename.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
    rename.restype = ctypes.c_int
    directory = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for name, data in (("staged", b"new"), ("occupied", b"old")):
            fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600, dir_fd=directory)
            try:
                require(os.write(fd, data) == len(data), "short fixture write")
                os.fsync(fd)
                fcntl.fcntl(fd, fcntl.F_FULLFSYNC)
            finally:
                os.close(fd)
        # RENAME_EXCL=0x4 from the installed Darwin SDK stdio.h.
        require(rename(directory, b"staged", directory, b"occupied", 4) == -1
                and ctypes.get_errno() == errno.EEXIST, "no-clobber rejected incorrectly")
        require((root / "occupied").read_bytes() == b"old", "destination changed")
        require(rename(directory, b"staged", directory, b"published", 4) == 0, "publication failed")
        os.fsync(directory)
        require((root / "published").read_bytes() == b"new", "published bytes differ")
        results["publication"] = "exclusive rename, file FULLFSYNC, directory fsync passed; no power-loss proof"
    finally:
        os.close(directory)

    lock_path = root / "stable.lock"
    lock = os.open(lock_path, os.O_CREAT | os.O_EXCL | os.O_RDWR | os.O_CLOEXEC, 0o600)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        lock_child = "import fcntl,os,sys; f=os.open(sys.argv[1],os.O_RDWR);\ntry: fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB)\nexcept BlockingIOError: sys.exit(17)\nsys.exit(0)"
        blocked = subprocess.run([sys.executable, "-c", lock_child, str(lock_path)], env=env, timeout=5, close_fds=True)
        require(blocked.returncode == 17, "independent lock did not contend")
        # Closing another independently opened descriptor must not drop this lock.
        os.close(os.open(lock_path, os.O_RDONLY | os.O_CLOEXEC))
        blocked = subprocess.run([sys.executable, "-c", lock_child, str(lock_path)], env=env, timeout=5, close_fds=True)
        require(blocked.returncode == 17, "unrelated close dropped lock")
        fcntl.flock(lock, fcntl.LOCK_UN)
        released = subprocess.run([sys.executable, "-c", lock_child, str(lock_path)], env=env, timeout=5, close_fds=True)
        require(released.returncode == 0, "lock did not release")
        results["locking"] = "flock contention/unrelated-close/release passed; not OFD-lock equivalence or crash proof"
    finally:
        os.close(lock)

    # Owned, inert Mach-O fixtures only: demonstrate why pathname launch is insufficient.
    source = root / "fixture.c"
    source.write_text("int main(void) { return FIXTURE; }\n")
    for name, marker in (("checked", 31), ("replacement", 32)):
        subprocess.run(["/usr/bin/clang", "-DFIXTURE=" + str(marker), str(source), "-o", str(root/name)],
                       check=True, env=env, timeout=60, capture_output=True)
    image = os.open(root / "checked", os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW)
    try:
        require(os.read(image, 4) == bytes.fromhex("cffaedfe"), "not little-endian Mach-O64")
        before = os.fstat(image)
        os.replace(root / "replacement", root / "checked")
        after = os.stat(root / "checked", follow_symlinks=False)
        require((before.st_dev, before.st_ino) != (after.st_dev, after.st_ino), "fixture identity unchanged")
        launched = subprocess.run([str(root / "checked")], env=env, timeout=5, close_fds=True)
        require(launched.returncode == 32, "replacement did not run")
        results["execution"] = "counterexample confirmed: verified open inode is not the later pathname-launched image"
    finally:
        os.close(image)

    # Two direct children, both retained and reaped: no orphan or real manager execution.
    leader = None
    escape = None
    try:
        leader = subprocess.Popen([sys.executable, "-c", "import os,signal; os.setpgid(0,0); print('ready',flush=True); signal.pause()"],
                                  env=env, stdout=subprocess.PIPE, text=True, close_fds=True)
        import selectors
        def ready(child):
            with selectors.DefaultSelector() as select:
                select.register(child.stdout, selectors.EVENT_READ)
                require(bool(select.select(5)), "child readiness timeout")
                require(child.stdout.readline().strip() == "ready", "child readiness failed")
        ready(leader)
        escape = subprocess.Popen([sys.executable, "-c", "import os,signal,sys; os.setpgid(0,int(sys.argv[1])); os.setsid(); print('ready',flush=True); signal.pause()", str(leader.pid)],
                                  env=env, stdout=subprocess.PIPE, text=True, close_fds=True)
        ready(escape)
        os.killpg(leader.pid, signal.SIGTERM)
        require(leader.wait(timeout=5) == -signal.SIGTERM, "group leader did not terminate")
        require(escape.poll() is None, "escape fixture unexpectedly terminated")
        results["lifecycle"] = "counterexample confirmed: process-group termination misses setsid escape; both owned children reaped"
    finally:
        for child in (escape, leader):
            if child is not None:
                if child.poll() is None:
                    child.kill()
                child.wait(timeout=5)
                child.stdout.close()
    return results


if __name__ == "__main__":
    require(platform.system() == "Darwin" and platform.machine() == "arm64", "Darwin arm64 required")
    require(len(sys.argv) == 2, "supply an existing disposable APFS parent directory")
    parent = Path(sys.argv[1]).resolve(strict=True)
    require(parent.is_dir(), "parent must be a directory")
    with tempfile.TemporaryDirectory(prefix="zerb-darwin-primitive-", dir=parent) as tmp:
        os.chmod(tmp, 0o700)
        results = run(Path(tmp))
    print(json.dumps({"diagnostic_only": True, "host_qualified": False, "release_supported": False,
                      "platform": platform.mac_ver()[0], "architecture": platform.machine(), "results": results}, indent=2))
