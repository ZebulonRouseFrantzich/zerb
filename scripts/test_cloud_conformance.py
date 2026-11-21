"""Boundary tests for local cloud evidence; these do not exercise host proofs."""

import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
import cloud_conformance as cloud


def valid_report():
    identity = {"path": "/test/tool", "type": "regular", "device": 1, "inode": 2,
                "size": 10, "mode": 0o755, "uid": 1, "gid": 1,
                "mtime_ns": 1, "ctime_ns": 1, "sha256": "a" * 64, "version": "go1.24.9"}
    ca = {"status": "passed", "identity": {**identity, "size": cloud.CA_SIZE, "sha256": cloud.CA_SHA256}}
    source = {"status": "passed", "commit": "b" * 40, "dirty": True,
              "tree_sha256": "c" * 64, "index_sha256": "d" * 64,
              "status_sha256": "e" * 64, "file_count": 1}
    fs = {"path": "/tmp", "mountpoint": "/", "filesystem": "overlay", "mount_device": "0:1",
          "type": "directory", "read_only": False, "device": 1, "inode": 1}
    inventory = {"status": "passed", "kernel": {"system": "Linux", "release": "other",
                 "version": "test", "machine": "x86_64"},
                 "distribution": {"ID": "debian", "VERSION_ID": "12"},
                 "filesystems": {key: copy.deepcopy(fs) for key in ("repository", "output_parent", "temporary")},
                 "capabilities": {key: {"status": "not-exercised"} for key in ("cgroup_v2", "btrfs", "proc", "ptmx")},
                 "candidate_host_match": {"status": "failed", "qualification": "not-exercised",
                     "fields": {key: False for key in ("distribution", "kernel", "architecture", "repository_filesystem")}}}
    inventory["capabilities"]["cgroup_v2"].update({"mount_present": False, "mount_read_only": None,
        "delegation": "not-exercised", "controllers": None, "subtree_control": None, "current_membership": None})
    inventory["capabilities"]["btrfs"]["kernel_registered"] = None
    inventory["capabilities"]["proc"].update({"mount_present": False, "filesystem": None})
    inventory["capabilities"]["ptmx"]["identity"] = None
    inventory["candidate_host_match"]["fields"]["architecture"] = True
    inventory["capabilities"]["namespaces"] = {key: {"status": "not-exercised", "identity": None}
        for key in ("user", "mnt", "pid", "net", "cgroup", "ipc", "uts", "time")}
    tools = {key: {"status": "passed", "identity": copy.deepcopy(identity)} for key in cloud.REQUIRED_TOOLS}
    for name in set(cloud.TOOL_ARGS) - set(cloud.REQUIRED_TOOLS):
        tools[name] = {"status": "failed"}
    for name, version in cloud.CANDIDATE_VERSIONS.items():
        tools[name].update({"candidate_version": version, "candidate_match": {
            "status": "not-exercised" if tools[name]["status"] == "passed" else "failed",
            "version_match": "failed", "identity_qualification": "not-exercised",
            "provenance_and_frozen_digest_match": "not-exercised"}})
    return {"schema": cloud.SCHEMA, "host_qualified": False, "release_supported": False,
            "inventory_only": False, "source": {"before": source, "after": copy.deepcopy(source)},
            "ca": {"before": ca, "after": copy.deepcopy(ca)}, "inventory": inventory,
            "tools": {"before": tools, "after": copy.deepcopy(tools)},
            "stability": {"status": "passed"},
            "checks": [{"name": name, "status": "passed", "exit_code": 0} for name in cloud.REQUIRED_CHECKS],
            "host_execution": {name: {"status": "not-exercised"} for name in cloud.HOST_ROWS}}


class GateTests(unittest.TestCase):
    def test_unsupported_host_and_missing_optional_tool_can_pass_only_cloud(self):
        report = valid_report()
        self.assertEqual(cloud.gate_status(report), "passed")
        self.assertIs(report["host_qualified"], False)
        self.assertIs(report["release_supported"], False)

    def test_inventory_only_cannot_pass(self):
        report = valid_report()
        report["inventory_only"] = True
        self.assertEqual(cloud.gate_status(report), "failed")
        report["checks"] = [{"name": name, "status": "not-exercised"} for name in cloud.REQUIRED_CHECKS]
        self.assertEqual(cloud.gate_status(report), "not-exercised")

    def test_missing_unknown_failed_and_lying_check_statuses_fail(self):
        for value in (None, "unknown", "pass", "failed", "not-exercised", True):
            with self.subTest(value=value):
                report = valid_report()
                report["checks"][0]["status"] = value
                self.assertEqual(cloud.gate_status(report), "failed")
        report = valid_report()
        report["checks"][0]["exit_code"] = 7
        self.assertEqual(cloud.gate_status(report), "failed")

    def test_omitted_duplicate_and_extra_checks_fail(self):
        for change in (lambda rows: rows.pop(), lambda rows: rows.append(rows[0]),
                       lambda rows: rows.__setitem__(0, rows[1]),
                       lambda rows: rows[0].__setitem__("name", "other")):
            report = valid_report()
            change(report["checks"])
            self.assertEqual(cloud.gate_status(report), "failed")

    def test_missing_and_malformed_critical_identity_fail(self):
        for section in ("source", "ca", "tools", "inventory", "stability"):
            report = valid_report()
            del report[section]
            self.assertEqual(cloud.gate_status(report), "failed")
        for key, value in (("inode", None), ("sha256", "wrong"), ("path", "relative"),
                           ("type", "symlink"), ("version", "")):
            report = valid_report()
            for label in ("before", "after"):
                report["tools"][label]["go"]["identity"][key] = value
            self.assertEqual(cloud.gate_status(report), "failed")

    def test_missing_required_tool_fails(self):
        for name in cloud.REQUIRED_TOOLS:
            report = valid_report()
            for label in ("before", "after"):
                report["tools"][label][name] = {"status": "failed"}
            self.assertEqual(cloud.gate_status(report), "failed")

    def test_wrong_go_family_fails(self):
        for version in ("go1.23.9", "go1.25.1", "go1.24rc1", "unknown"):
            report = valid_report()
            for label in ("before", "after"):
                report["tools"][label]["go"]["identity"]["version"] = version
            self.assertEqual(cloud.gate_status(report), "failed")

    def test_lying_stability_cannot_hide_identity_changes(self):
        for section in ("source", "ca", "tools"):
            report = valid_report()
            report[section]["after"]["changed"] = True
            self.assertEqual(cloud.gate_status(report), "failed")

    def test_ca_size_or_hash_substitution_fails(self):
        for key, value in (("size", cloud.CA_SIZE - 1), ("sha256", "0" * 64)):
            report = valid_report()
            for label in ("before", "after"):
                report["ca"][label]["identity"][key] = value
            self.assertEqual(cloud.gate_status(report), "failed")

    def test_host_claims_and_execution_statuses_cannot_be_promoted(self):
        for key in ("host_qualified", "release_supported"):
            report = valid_report()
            report[key] = True
            self.assertEqual(cloud.gate_status(report), "failed")
        report = valid_report()
        report["host_execution"][cloud.HOST_ROWS[0]]["status"] = "passed"
        self.assertEqual(cloud.gate_status(report), "failed")

    def test_missing_or_lying_inventory_fails(self):
        for key in ("kernel", "distribution", "filesystems", "capabilities"):
            report = valid_report()
            report["inventory"][key] = {"status": "passed"}
            self.assertEqual(cloud.gate_status(report), "failed")
        report = valid_report()
        report["inventory"]["capabilities"]["btrfs"]["status"] = "passed"
        self.assertEqual(cloud.gate_status(report), "failed")

    def test_capability_fields_are_required_and_typed(self):
        samples = valid_report()["inventory"]["capabilities"]
        for capability in ("cgroup_v2", "btrfs", "proc", "ptmx"):
            for field in samples[capability]:
                with self.subTest(capability=capability, field=field):
                    report = valid_report()
                    del report["inventory"]["capabilities"][capability][field]
                    self.assertEqual(cloud.gate_status(report), "failed")
        for capability, field, value in (("cgroup_v2", "mount_present", "unknown"),
                ("cgroup_v2", "controllers", [True]), ("cgroup_v2", "delegation", "passed"),
                ("btrfs", "kernel_registered", "unknown"), ("proc", "filesystem", False),
                ("ptmx", "identity", {"status": "passed"})):
            report = valid_report()
            report["inventory"]["capabilities"][capability][field] = value
            self.assertEqual(cloud.gate_status(report), "failed")
        for value in ({"status": "not-exercised"}, {"status": "not-exercised", "identity": "unknown"}):
            report = valid_report()
            report["inventory"]["capabilities"]["namespaces"]["user"] = value
            self.assertEqual(cloud.gate_status(report), "failed")
        report = valid_report()
        del report["inventory"]["candidate_host_match"]
        self.assertEqual(cloud.gate_status(report), "failed")

    def test_optional_tool_observation_cannot_be_omitted_or_lie(self):
        for name in set(cloud.TOOL_ARGS) - set(cloud.REQUIRED_TOOLS):
            report = valid_report()
            for label in ("before", "after"):
                del report["tools"][label][name]
            self.assertEqual(cloud.gate_status(report), "failed")
        report = valid_report()
        for label in ("before", "after"):
            report["tools"][label]["bash"]["status"] = "passed"
            report["tools"][label]["bash"]["candidate_match"]["status"] = "not-exercised"
        self.assertEqual(cloud.gate_status(report), "failed")

    def test_candidate_host_match_must_equal_recorded_host_facts(self):
        report = valid_report()
        match = report["inventory"]["candidate_host_match"]
        match["fields"] = {key: True for key in match["fields"]}
        match["status"] = "passed"
        self.assertEqual(cloud.gate_status(report), "failed")
        for field in ("status", "version_match", "identity_qualification", "provenance_and_frozen_digest_match"):
            report = valid_report()
            for label in ("before", "after"):
                report["tools"][label]["mise"]["candidate_match"][field] = "passed"
            self.assertEqual(cloud.gate_status(report), "failed")


class FilesystemTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="cloud-boundary-", dir="/tmp")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)

    def test_strict_ca_fixture(self):
        repo = Path(cloud.__file__).resolve().parent.parent
        self.assertEqual(cloud.ca_identity(repo)["status"], "passed")
        target = self.root / cloud.CA_PATH
        target.parent.mkdir(parents=True)
        target.write_bytes(b"wrong")
        self.assertEqual(cloud.ca_identity(self.root)["status"], "failed")
        target.unlink()
        target.symlink_to(repo / cloud.CA_PATH)
        self.assertEqual(cloud.ca_identity(self.root)["status"], "failed")

    def test_ca_symlink_ancestor_rejected(self):
        original = Path(cloud.__file__).resolve().parent.parent
        (self.root / "internal").symlink_to(original / "internal", target_is_directory=True)
        self.assertEqual(cloud.ca_identity(self.root)["status"], "failed")

    def test_changed_open_file_rejected(self):
        path = self.root / "input"
        path.write_bytes(b"before")
        original_read = os.read
        changed = False

        def read(fd, size):
            nonlocal changed
            data = original_read(fd, size)
            if not changed:
                changed = True
                path.write_bytes(b"after!")
            return data

        with mock.patch.object(cloud.os, "read", side_effect=read):
            with self.assertRaises(cloud.EvidenceError):
                cloud.file_identity(path)

    def test_safe_output_exclusive_outside_repo(self):
        repo = self.root / "repo"
        repo.mkdir()
        with self.assertRaises(cloud.EvidenceError):
            cloud.reserve_output(repo / "report.json", repo)
        path = self.root / "report.json"
        os.close(cloud.reserve_output(path, repo))
        self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        with self.assertRaises(FileExistsError):
            cloud.reserve_output(path, repo)
        path.unlink()
        path.symlink_to(repo / "victim")
        with self.assertRaises(FileExistsError):
            cloud.reserve_output(path, repo)
        link = self.root / "linked"
        link.symlink_to(repo, target_is_directory=True)
        with self.assertRaises(cloud.EvidenceError):
            cloud.reserve_output(link / "output.json", repo)

    def test_output_protected_home_xdg_and_moved_parent_rejected(self):
        repo = self.root / "repo"
        repo.mkdir()
        protected = self.root / "protected"
        protected.mkdir()
        for key in ("HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME"):
            with self.subTest(key=key), mock.patch.dict(os.environ, {key: str(protected)}):
                with self.assertRaises(cloud.EvidenceError):
                    cloud.reserve_output(protected / "report.json", repo)
        with mock.patch.object(cloud, "protected_roots", return_value=[protected]), \
                mock.patch.object(cloud.os, "readlink", return_value=str(protected)):
            with self.assertRaises(cloud.EvidenceError):
                cloud.reserve_output(self.root / "report.json", repo)

    def test_command_nonzero_is_failed_and_diagnostics_are_redacted(self):
        env = cloud.environment(self.root, os.defpath)
        result = cloud.run_check("test", [sys.executable, "-c",
            "import sys; print('credential-sentinel'); sys.exit(7)"], self.root, env, self.root)
        self.assertEqual(result["status"], "failed")
        self.assertEqual(result["exit_code"], 7)
        # argv is declared input; subprocess output itself is omitted.
        result.pop("argv")
        self.assertNotIn("credential-sentinel", json.dumps(result))

    def test_child_environment_is_disposable_and_readonly(self):
        with mock.patch.dict(os.environ, {"CREDENTIAL_SENTINEL": "private", "HOME": "/real-home"}):
            env = cloud.environment(self.root, os.defpath)
        self.assertNotIn("CREDENTIAL_SENTINEL", env)
        self.assertEqual(env["HOME"], str(self.root / "home"))
        self.assertEqual(env["GOFLAGS"], "-mod=readonly -buildvcs=false")
        self.assertEqual(env["GOPROXY"], "off")
        self.assertEqual(env["PYTHONDONTWRITEBYTECODE"], "1")

    def test_unsafe_ambient_build_cache_paths_fail(self):
        repo = self.root / "repo"
        repo.mkdir()
        account_home = self.root / "account-home"
        account_home.mkdir()
        cache = account_home / "cache"
        cache.mkdir()
        for candidate in (str(repo), str(cache), "relative-cache", str(self.root / "missing")):
            with self.subTest(candidate=candidate), mock.patch.dict(os.environ,
                    {"HOME": str(account_home), "GOCACHE": candidate}, clear=True):
                with self.assertRaises(cloud.EvidenceError):
                    cloud.environment(self.root, os.defpath, repo)

    def test_stacked_mount_uses_observed_device_and_top_mount(self):
        info = self.root.stat()
        device = str(os.major(info.st_dev)) + ":" + str(os.minor(info.st_dev))
        rows = [{"mount_id": 1, "mountpoint": "/", "mount_device": "999:999",
                 "filesystem": "wrong", "read_only": True},
                {"mount_id": 91, "mountpoint": str(self.root), "mount_device": device,
                 "filesystem": "tmpfs", "read_only": True},
                {"mount_id": 106, "mountpoint": str(self.root), "mount_device": device,
                 "filesystem": "tmpfs", "read_only": False}]
        result = cloud.filesystem_identity(self.root, rows)
        self.assertFalse(result["read_only"])
        self.assertEqual(result["mount_id"], 106)

    def test_missing_optional_tools_are_failed_identities(self):
        env = cloud.environment(self.root, os.defpath)
        with mock.patch.object(cloud.shutil, "which", return_value=None):
            tools = cloud.tool_inventory(self.root, env)
        self.assertEqual(tools["mise"]["status"], "failed")
        self.assertEqual(tools["mise"]["candidate_match"]["status"], "failed")

    def test_actual_collector_python_is_measured_instead_of_path_alias(self):
        env = cloud.environment(self.root, os.defpath)
        original_which = cloud.shutil.which

        def which(name, path):
            return "/untrusted/other-python" if name == "python3" else original_which(name, path=path)

        with mock.patch.object(cloud.shutil, "which", side_effect=which):
            tools = cloud.tool_inventory(self.root, env)
        self.assertEqual(tools["python3"]["status"], "passed")
        self.assertEqual(tools["python3"]["identity"]["path"], str(Path(sys.executable).resolve()))

    def test_protected_tmp_fails_before_any_output_or_temporary_creation(self):
        output = self.root / "report.json"
        with mock.patch.dict(os.environ, {"XDG_DATA_HOME": "/tmp"}), \
                mock.patch.object(cloud.tempfile, "TemporaryDirectory") as create_temp, \
                mock.patch.object(cloud, "reserve_output") as reserve:
            self.assertEqual(cloud.main(["--output", str(output), "--inventory-only"]), 1)
        create_temp.assert_not_called()
        reserve.assert_not_called()
        self.assertFalse(output.exists())

    def test_fixed_command_set(self):
        tools = {name: {"identity": {"path": "/measured/" + name}} for name in cloud.REQUIRED_TOOLS}
        commands = dict(cloud.commands(tools, self.root, self.root))
        self.assertEqual(set(commands), set(cloud.REQUIRED_CHECKS))
        self.assertEqual(commands["proof-race"], ["/measured/go", "test", "-race", "-count=1", "./internal/proof/..."])
        self.assertIn("./internal/paths", commands["foundation-race"])
        self.assertIn("./internal/safefs", commands["foundation-race"])
        self.assertIn("./internal/toolchain", commands["foundation-race"])
        self.assertIn("./internal/state", commands["foundation-race"])
        self.assertIn("./internal/execenv", commands["foundation-race"])
        self.assertIn("-trimpath", commands["release-build"])
        self.assertEqual(commands["release-build"][-1], "./cmd/zerb")
        self.assertIn("test_*.py", commands["python-boundaries"])
        self.assertEqual(commands["parser-pin"], ["/measured/python3", "scripts/check_parser_pin.py", "--go", "/measured/go"])
        self.assertEqual(commands["parser-upstream-race"], ["/measured/go", "-C", "third_party/go-toml", "test", "-race", "-count=1", "./..."])


class SourceTests(unittest.TestCase):
    def test_commit_dirty_index_and_ignored_content_are_bound(self):
        with tempfile.TemporaryDirectory(prefix="cloud-source-", dir="/tmp") as directory:
            root = Path(directory)
            repo = root / "repo"
            repo.mkdir()
            env = cloud.environment(root, os.environ.get("PATH", os.defpath))
            git = shutil.which("git", path=env["PATH"])
            if not git:
                self.skipTest("Git unavailable")

            def run(*args):
                subprocess.run([git, *args], cwd=repo, env=env, stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL, check=True)

            run("init")
            (repo / "input").write_text("one")
            (repo / ".gitignore").write_text("ignored\n")
            run("add", ".")
            run("-c", "user.name=Cloud Test", "-c", "user.email=cloud@example.invalid", "commit", "-m", "fixture")
            clean = cloud.source_identity(repo, git, env)
            self.assertFalse(clean["dirty"])
            self.assertEqual(clean, cloud.source_identity(repo, git, env))
            (repo / "input").write_text("two")
            dirty = cloud.source_identity(repo, git, env)
            self.assertTrue(dirty["dirty"])
            self.assertEqual(clean["commit"], dirty["commit"])
            self.assertNotEqual(clean["tree_sha256"], dirty["tree_sha256"])
            (repo / "untracked.py").write_text("change")
            untracked = cloud.source_identity(repo, git, env)
            self.assertNotEqual(dirty["tree_sha256"], untracked["tree_sha256"])
            (repo / "ignored").write_text("bound despite ignore")
            ignored = cloud.source_identity(repo, git, env)
            self.assertEqual(untracked["status_sha256"], ignored["status_sha256"])
            self.assertNotEqual(untracked["tree_sha256"], ignored["tree_sha256"])
            run("add", "input")
            staged = cloud.source_identity(repo, git, env)
            self.assertNotEqual(ignored["index_sha256"], staged["index_sha256"])
            # Bind no source through links, even if the target is readable or a
            # directory. The external target bytes must never be opened.
            external = root / "external"
            external.write_text("unbound content")
            (repo / "linked.py").symlink_to(external)
            with self.assertRaises(cloud.EvidenceError):
                cloud.source_identity(repo, git, env)
            (repo / "linked.py").unlink()
            (repo / "linked-directory").symlink_to(root, target_is_directory=True)
            with self.assertRaises(cloud.EvidenceError):
                cloud.source_identity(repo, git, env)

    def test_gitlink_pointer_is_bound_without_loading_submodule(self):
        with tempfile.TemporaryDirectory(prefix="cloud-gitlink-", dir="/tmp") as directory:
            root = Path(directory)
            repo = root / "repo"
            repo.mkdir()
            env = cloud.environment(root, os.environ.get("PATH", os.defpath))
            git = shutil.which("git", path=env["PATH"])
            if not git:
                self.skipTest("Git unavailable")
            for args in (["init"], ["-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                          "commit", "--allow-empty", "-m", "fixture"],
                         ["update-index", "--add", "--cacheinfo", "160000," + "a" * 40 + ",submodule"]):
                subprocess.run([git, *args], cwd=repo, env=env, stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL, check=True)
            (repo / "submodule").mkdir()
            self.assertEqual(cloud.source_identity(repo, git, env)["status"], "passed")


if __name__ == "__main__":
    unittest.main()
