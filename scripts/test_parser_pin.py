"""Disposable integrity/selection fixtures for the PC-042 parser pin checker."""

import contextlib
import copy
import io
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
import check_parser_pin as pin


REPOSITORY = Path(__file__).resolve().parents[1]


class HashTests(unittest.TestCase):
    def test_invalid_arguments_have_constant_diagnostics(self):
        for arguments in ([], ["--unexpected", "PrivateMetadataMarker"]):
            with self.subTest(arguments=arguments):
                error = io.StringIO()
                with contextlib.redirect_stderr(error), self.assertRaises(SystemExit) as outcome:
                    pin.main(arguments)
                self.assertEqual(outcome.exception.code, 2)
                self.assertEqual(error.getvalue(), "parser pin: invalid arguments\n")

    def test_independent_go_hash1_golden_vector(self):
        files = {"dir/b.txt": "b413f47d13ee2fe6c845b2ee141af81de858df4ec549a58b7970bb96645bc8d2",
                 "a.txt": "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"}
        self.assertEqual(pin.hash1(files, "example.org/mod@v1.2.3/"),
                         "h1:UsutkFbtyoad3fTqIZRyUEW8LIHZhNN9WxyosK/uGdk=")
        self.assertEqual(pin.hash1({}), "h1:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=")

    def test_official_upstream_inventory_and_go_mod_hashes(self):
        manifest = json.loads((REPOSITORY / pin.MANIFEST_FILE).read_bytes())
        files = manifest["upstream"]["files"]
        self.assertEqual(len(files), 113)
        self.assertEqual(pin.hash1(files, pin.MODULE + "@v2.4.3/"), pin.UPSTREAM_SUM)
        self.assertEqual(pin.hash1({"go.mod": files["go.mod"]}), pin.GO_MOD_SUM)


class PinTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="zerb-parser-pin-test-")
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.root = self.base / "repository"
        self.root.mkdir()
        self.fork = self.root / pin.FORK_DIR
        shutil.copytree(REPOSITORY / pin.FORK_DIR, self.fork)
        for name in (pin.MANIFEST_FILE, "third_party/" + pin.PATCH_FILE):
            shutil.copyfile(REPOSITORY / name, self.root / name)
        self.manifest_path = self.root / pin.MANIFEST_FILE
        self.provenance = json.loads(self.manifest_path.read_bytes())
        self.mod = self.root / "go.mod"
        self.mod.write_text("module example.org/disposable\n\ngo 1.24.0\n"
                            f"require {pin.MODULE} {pin.VERSION}\n"
                            f"replace {pin.MODULE} {pin.VERSION} => ./third_party/go-toml\n")
        self.selection = {"Path": pin.MODULE, "Version": pin.VERSION, "Dir": str(self.fork),
                          "Replace": {"Path": "./third_party/go-toml", "Dir": str(self.fork)}}
        self.go = self.base / "go"
        self.fake_go()
        environment = {"HOME": str(self.base / "home"), "XDG_CONFIG_HOME": str(self.base / "config"),
                       "XDG_CACHE_HOME": str(self.base / "cache"), "XDG_DATA_HOME": str(self.base / "data"),
                       "XDG_STATE_HOME": str(self.base / "state"), "GOPROXY": "poisoned",
                       "GOSUMDB": "poisoned", "GOFLAGS": "-mod=mod", "GOWORK": "poisoned",
                       "GOENV": "poisoned", "GOTOOLCHAIN": "poisoned", "GONOPROXY": "*",
                       "GOPRIVATE": "*", "GONOSUMDB": "*", "GOVCS": "*:all", "GOAUTH": "netrc"}
        self.environ = mock.patch.dict(os.environ, environment)
        self.environ.start()
        self.addCleanup(self.environ.stop)

    def fake_go(self, selection=None, status=0, raw=None):
        expected_env = {"GOFLAGS": "-mod=readonly -buildvcs=false", "GOPROXY": "off", "GOSUMDB": "off",
                        "GONOPROXY": "none", "GOPRIVATE": "", "GONOSUMDB": "none",
                        "GOVCS": "*:off", "GOAUTH": "off", "GOTOOLCHAIN": "local",
                        "GOWORK": "off", "GOENV": "off"}
        output = json.dumps(self.selection if selection is None else selection) if raw is None else raw
        self.go.write_text(f"#!{sys.executable}\nimport os,sys\n"
                           f"expected = {expected_env!r}\n"
                           f"assert sys.argv[1:] == {['list', '-m', '-json', pin.MODULE]!r}\n"
                           f"assert os.getcwd() == {str(self.root)!r}\n"
                           "assert all(os.environ.get(k) == v for k,v in expected.items())\n"
                           f"sys.stdout.write({output!r})\n"
                           f"sys.stderr.write('PrivateChildOutput')\nsys.exit({status})\n")
        self.go.chmod(0o700)

    def update_manifest(self, change):
        change(self.provenance)
        self.manifest_path.write_text(json.dumps(self.provenance))

    def refuse(self):
        with self.assertRaises(pin.PinError):
            pin.check(self.root, str(self.go))

    def test_complete_fixture_passes_and_is_read_only(self):
        before = {str(path.relative_to(self.root)): path.read_bytes()
                  for path in self.root.rglob("*") if path.is_file()}
        pin.check(self.root, str(self.go))
        after = {str(path.relative_to(self.root)): path.read_bytes()
                 for path in self.root.rglob("*") if path.is_file()}
        self.assertEqual(before, after)

    def test_hidden_upstream_file_is_required(self):
        (self.fork / ".dockerignore").unlink()
        self.refuse()

    def test_missing_source(self):
        (self.fork / "unstable/parser.go").unlink()
        self.refuse()

    def test_missing_added_test(self):
        (self.fork / pin.ADDED_TESTS[0]).unlink()
        self.refuse()

    def test_extra_file(self):
        (self.fork / "extra.go").write_text("package toml\n")
        self.refuse()

    def test_extra_empty_directory(self):
        (self.fork / "extra-empty").mkdir()
        self.refuse()

    def test_altered_original_file(self):
        (self.fork / "unstable/parser.go").write_text("package unstable\n")
        self.refuse()

    def test_altered_patch_file(self):
        (self.root / "third_party" / pin.PATCH_FILE).write_text("PrivateMetadataMarker")
        self.refuse()

    def test_altered_patched_source_even_with_updated_module_sum(self):
        (self.fork / "unmarshaler.go").write_text("package toml\n")
        self.update_manifest(lambda manifest: manifest.update(module_sum=pin.hash1(pin.fork_files(self.root), pin.MODULE + "@" + pin.VERSION + "/")))
        self.refuse()

    def test_altered_added_test_mismatches_pin(self):
        (self.fork / pin.ADDED_TESTS[0]).write_text("package toml\n")
        self.refuse()

    def test_altered_added_test_cannot_repin_manifest(self):
        (self.fork / pin.ADDED_TESTS[0]).write_text("package toml\n")
        self.update_manifest(lambda manifest: manifest.update(module_sum=pin.hash1(pin.fork_files(self.root), pin.MODULE + "@" + pin.VERSION + "/")))
        self.refuse()

    def test_altered_go_mod(self):
        (self.fork / "go.mod").write_text("module example.org/renamed\n")
        self.refuse()

    def test_upstream_file_inventory_cannot_be_rewritten(self):
        self.update_manifest(lambda manifest: manifest["upstream"]["files"].update({"unstable/parser.go": "0" * 64}))
        self.refuse()

    def test_manifest_unknown_top_level_key(self):
        self.update_manifest(lambda manifest: manifest.update(unknown=True))
        self.refuse()

    def test_manifest_unknown_nested_key(self):
        self.update_manifest(lambda manifest: manifest["upstream"].update(unknown=True))
        self.refuse()

    def test_manifest_unknown_patch_key(self):
        self.update_manifest(lambda manifest: manifest["patch"].update(unknown=True))
        self.refuse()

    def test_manifest_wrong_fixed_identity(self):
        self.update_manifest(lambda manifest: manifest["upstream"].update(origin_commit="0" * 40))
        self.refuse()

    def test_manifest_schema_boolean_is_not_integer_one(self):
        self.update_manifest(lambda manifest: manifest.update(schema=True))
        self.refuse()

    def test_manifest_duplicate_key(self):
        self.manifest_path.write_text(self.manifest_path.read_text().replace('"schema": 1', '"schema": 1, "schema": 1'))
        self.refuse()

    def test_manifest_non_json_constant(self):
        self.manifest_path.write_text(self.manifest_path.read_text().replace('"schema": 1', '"schema": NaN'))
        self.refuse()

    def test_manifest_wrong_source_checksum(self):
        self.update_manifest(lambda manifest: manifest.update(module_sum="h1:" + "A" * 43 + "="))
        self.refuse()

    def test_manifest_path_traversal(self):
        def mutate(manifest):
            files = manifest["upstream"]["files"]
            files["../outside"] = files.pop("unmarshaler.go")
        self.update_manifest(mutate)
        self.refuse()

    def test_root_pin_requires_exact_version_specific_replacement(self):
        original = self.mod.read_text()
        variants = [original.replace(pin.VERSION, "v2.4.3", 1),
                    original.replace(f"replace {pin.MODULE} {pin.VERSION}", f"replace {pin.MODULE}"),
                    original.replace("./third_party/go-toml", "./outside"),
                    original + f"require {pin.MODULE} v2.4.2\n",
                    original + f"replace {pin.MODULE} v2.4.3 => ./third_party/go-toml\n",
                    original + f"exclude {pin.MODULE} {pin.VERSION}\n",
                    original.replace("require ", "// require "),
                    original.replace("replace ", "// replace ")]
        for value in variants:
            with self.subTest(value=value):
                self.mod.write_text(value)
                self.refuse()

    def test_root_pin_blocks_quoted_tokens_and_comments(self):
        self.mod.write_text(f'module example.org/disposable\nrequire (\n "{pin.MODULE}" "{pin.VERSION}" // indirect\n)\n'
                            f'replace (\n "{pin.MODULE}" "{pin.VERSION}" => "./third_party/go-toml"\n)\n')
        pin.check(self.root, str(self.go))

    def test_effective_selection_rejects_mvs_override(self):
        selected = copy.deepcopy(self.selection)
        selected["Version"] = "v2.4.4"
        self.fake_go(selected)
        self.refuse()

    def test_effective_selection_rejects_module_substitution(self):
        selected = copy.deepcopy(self.selection)
        selected["Path"] = "example.org/other"
        self.fake_go(selected)
        self.refuse()

    def test_effective_selection_rejects_other_replacement(self):
        for key, value in (("Path", "./outside"), ("Dir", str(self.base)), ("Version", pin.VERSION)):
            with self.subTest(key=key):
                selected = copy.deepcopy(self.selection)
                selected["Replace"][key] = value
                self.fake_go(selected)
                self.refuse()

    def test_effective_selection_rejects_module_dir_mismatch(self):
        selected = copy.deepcopy(self.selection)
        selected["Dir"] = str(self.base)
        self.fake_go(selected)
        self.refuse()

    def test_effective_selection_rejects_no_replacement(self):
        selected = copy.deepcopy(self.selection)
        del selected["Replace"]
        self.fake_go(selected)
        self.refuse()

    def test_go_command_error(self):
        self.fake_go(status=3)
        self.refuse()

    def test_go_command_timeout(self):
        with mock.patch.object(pin.subprocess, "run", side_effect=subprocess.TimeoutExpired("go", 60)):
            self.refuse()

    def test_go_command_cannot_start(self):
        with mock.patch.object(pin.subprocess, "run", side_effect=OSError("PrivateMetadataMarker")):
            self.refuse()

    def test_go_command_invalid_or_duplicate_json(self):
        for value in ("private invalid JSON", '{"Path":"x","Path":"y"}', "[]", "{}"):
            with self.subTest(value=value):
                self.fake_go(raw=value)
                self.refuse()

    def test_absolute_go_is_required(self):
        with self.assertRaises(pin.PinError):
            pin.check(self.root, "go")

    def test_missing_go_is_constant_cli_error(self):
        output = io.StringIO()
        with contextlib.redirect_stderr(output):
            self.assertEqual(pin.main(["--root", str(self.root), "--go", str(self.base / "missing")]), 1)
        self.assertEqual(output.getvalue(), "parser pin: verification failed\n")

    def test_noncanonical_ancestor_is_rejected(self):
        with self.assertRaises(pin.PinError):
            pin.check(self.root / "missing" / "..", str(self.go))

    def test_symlink_source_file(self):
        path = self.fork / "unmarshaler.go"
        target = self.base / "outside.go"
        target.write_bytes(path.read_bytes())
        path.unlink()
        path.symlink_to(target)
        self.refuse()

    def test_symlink_nested_directory(self):
        path = self.fork / "unstable"
        target = self.base / "outside"
        path.rename(target)
        path.symlink_to(target, target_is_directory=True)
        self.refuse()

    def test_symlink_fork_directory(self):
        target = self.base / "outside"
        self.fork.rename(target)
        self.fork.symlink_to(target, target_is_directory=True)
        self.refuse()

    def test_symlink_manifest(self):
        target = self.base / "outside.json"
        self.manifest_path.rename(target)
        self.manifest_path.symlink_to(target)
        self.refuse()

    def test_symlink_patch(self):
        path = self.root / "third_party" / pin.PATCH_FILE
        target = self.base / "outside.patch"
        path.rename(target)
        path.symlink_to(target)
        self.refuse()

    def test_symlink_ancestor(self):
        path = self.root / "third_party"
        target = self.base / "outside"
        path.rename(target)
        path.symlink_to(target, target_is_directory=True)
        self.refuse()

    def test_symlink_root(self):
        link = self.base / "linked-root"
        link.symlink_to(self.root, target_is_directory=True)
        with self.assertRaises(pin.PinError):
            pin.check(link, str(self.go))

    def test_nonregular_entry(self):
        os.mkfifo(self.fork / "fifo")
        self.refuse()

    def test_cli_constant_error_and_success(self):
        output, error = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(error):
            status = pin.main(["--root", str(self.root), "--go", str(self.go)])
        self.assertEqual(status, 0)
        self.assertIn("not host qualification", output.getvalue())
        self.assertEqual(error.getvalue(), "")
        self.fake_go(raw="PrivateChildOutput", status=1)
        output, error = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(error):
            status = pin.main(["--root", str(self.root), "--go", str(self.go)])
        self.assertEqual(status, 1)
        self.assertEqual(output.getvalue(), "")
        self.assertEqual(error.getvalue(), "parser pin: verification failed\n")


if __name__ == "__main__":
    unittest.main()
