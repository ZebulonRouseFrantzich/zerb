"""Intentional failing commands exercise retention without running host proofs."""

import io
import json
import os
from pathlib import Path
import sys
import subprocess
import tempfile
import unittest
from unittest import mock

import cloud_conformance as cloud
import cloud_diagnostics as diagnostics


class DiagnosticTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="zerb-diagnostics-", dir="/tmp")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        (self.repo / "go.mod").write_text("module example.test/proof\n")
        (self.repo / "fixture_test.go").write_text("package proof\nfunc TestFixture(t *testing.T) {\n t.Fatal(\"fixture\")\n}\n")
        self.sources = {name: (self.repo / name).read_bytes() for name in ("go.mod", "fixture_test.go")}
        self.catalog = diagnostics.source_catalog(self.sources)

    def record(self, action, **values):
        return {"Package": "example.test/proof", "Test": "TestFixture", "Action": action, **values}

    def collect(self, *records):
        raw = b"".join(json.dumps(row).encode() + b"\n" for row in records)
        return diagnostics.failure_diagnostics(io.BytesIO(raw), self.catalog)

    def trace(self):
        return {"role": "F", "mode": "held", "sequence": 1, "event": "wait",
                "phase": "status", "subject": "M", "raw_status": 137 << 8}

    def test_failed_command_checkpoint_retains_identity_status_order_not_values(self):
        sentinel = "must-not-persist"
        records = [self.record("run", Test="TestFixture/" + sentinel),
                   self.record("output", Output="    fixture_test.go:3: " + sentinel + "\n"),
                   self.record("output", Output="ZERB_ASSEMBLY_TRACE " + json.dumps(self.trace()) + "\n"),
                   self.record("fail")]
        script = "import sys;sys.stdout.write(" + repr("".join(json.dumps(row) + "\n" for row in records)) + ");sys.exit(7)"
        path = self.root / "failure.checks.jsonl"
        fd = cloud.reserve_output(path, self.repo)
        with os.fdopen(fd, "w") as stream:
            result = cloud.run_check("proof-race", [sys.executable, "-c", script], self.repo,
                                     cloud.environment(self.root, os.defpath), self.root,
                                     lambda row: cloud.diagnostic_checkpoint(stream, row), self.catalog)
        result.pop("argv")  # Declared command input is not tool diagnostic output.
        retained = json.dumps(result)
        self.assertNotIn(sentinel, retained)
        self.assertNotIn(sentinel, path.read_text())
        self.assertEqual(result["exit_code"], 7)
        self.assertEqual(result["status"], "failed")
        evidence = result["diagnostics"]
        self.assertEqual(evidence["status"], "catalogued")
        self.assertEqual(evidence["failures"][0]["test"], "TestFixture")
        self.assertEqual([r["stream_ordinal"] for r in evidence["observations"]], [1, 2, 3, 4])
        location = evidence["observations"][1]
        self.assertEqual((location["file"], location["line"]), ("fixture_test.go", 3))
        self.assertEqual(evidence["observations"][2]["observation"]["raw_status"], 137 << 8)
        self.assertEqual([json.loads(line)["event"] for line in path.read_text().splitlines()],
                         ["check-start", "check-complete"])
        self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        # The safe journal outlives the disposable raw log.
        self.assertEqual(sorted(p.name for p in self.root.iterdir()),
                         ["cache", "config", "data", "failure.checks.jsonl", "home", "repo", "state", "tmp"])

    def test_malformed_unknown_and_oversized_records_are_explicitly_incomplete(self):
        for row in (None, [], self.record("output", Output="ZERB_ASSEMBLY_TRACE {}"),
                    self.record("fail", Package="unreviewed-secret-package"),
                    self.record("fail", Test="SecretTest")):
            with self.subTest(row=row):
                result = self.collect(row)
                self.assertEqual(result["status"], "incomplete")
                self.assertNotIn("secret", json.dumps(result))
        raw = b"X" * (diagnostics.MAX_LINE + 1) + b"\n" + json.dumps(self.record("fail")).encode() + b"\n"
        result = diagnostics.failure_diagnostics(io.BytesIO(raw), self.catalog)
        self.assertEqual(result["status"], "incomplete")
        self.assertEqual(len(result["failures"]), 1)
        self.assertNotIn("XXXX", json.dumps(result))

    def test_typed_records_reject_extra_fields_types_bounds_and_duplicates(self):
        for key, value in (("role", "secret"), ("phase", {}), ("subject", []),
                           ("sequence", True), ("sequence", 2**64), ("raw_status", -1),
                           ("raw_status", 65536), ("raw_status", True), ("nonce", "secret")):
            with self.subTest(key=key, value=value):
                trace = self.trace()
                trace[key] = value
                self.assertFalse(diagnostics.assembly_record(trace))
        duplicate = b'{"Package":"example.test/proof","Action":"pass","Action":"fail"}\n'
        self.assertEqual(diagnostics.failure_diagnostics(io.BytesIO(duplicate), self.catalog)["status"], "incomplete")

    def test_truncation_preserves_first_failed_identity_and_admits_no_completeness_claim(self):
        with mock.patch.object(diagnostics, "MAX_EVENTS", 2):
            result = self.collect(self.record("output", Output="fixture_test.go:3: private"),
                                  self.record("fail"), *(self.record("run") for _ in range(4)))
        self.assertEqual(result["status"], "incomplete")
        self.assertEqual(len(result["observations"]), 2)
        self.assertEqual(result["failures"][0]["test"], "TestFixture")
        self.assertEqual(result["failures"][0]["location_ordinals"], [1])
        self.assertEqual(result["locations"][0]["line"], 3)

    def test_failed_case_cannot_borrow_another_cases_location(self):
        result = self.collect(self.record("output", Test="TestFixture/passed-secret-case", Output="fixture_test.go:3: private"),
                              self.record("pass", Test="TestFixture/passed-secret-case"),
                              self.record("fail", Test="TestFixture/failed-secret-case"),
                              self.record("fail", Test=""))
        self.assertEqual(result["status"], "incomplete")
        self.assertEqual(result["failures"][0]["location_ordinals"], [])
        self.assertEqual(result["missing_failure_locations"], 2)
        self.assertNotIn("secret-case", json.dumps(result))
        covered = self.collect(self.record("output", Output="fixture_test.go:3: private"),
                               self.record("fail"), self.record("fail", Test=""))
        self.assertEqual(covered["status"], "catalogued")
        self.assertEqual(covered["missing_failure_locations"], 0)

    def test_unknown_locations_and_symlink_sources_do_not_enter_catalog(self):
        (self.repo / "alias_test.go").symlink_to(self.repo / "fixture_test.go")
        result = self.collect(self.record("output", Output="../../secret.go:1: sensitive\nfixture_test.go:99999: sensitive"))
        self.assertEqual(result["observations"], [])
        self.assertNotIn("alias_test.go", self.catalog["example.test/proof"]["files"])

    def test_mutating_failing_command_cannot_expand_frozen_catalog(self):
        source = self.repo / "injected_test.go"
        secret = "SecretIdentifier"
        rows = [self.record("fail", Test="Test" + secret),
                self.record("output", Output="injected_test.go:1: hidden")]
        script = ("from pathlib import Path;import sys;Path(" + repr(str(source)) + ").write_text("
                  + repr("func Test" + secret + "(t *testing.T) {}") + ");sys.stdout.write("
                  + repr("".join(json.dumps(row) + "\n" for row in rows)) + ");sys.exit(1)")
        result = cloud.run_check("proof-race", [sys.executable, "-c", script], self.repo,
                                 cloud.environment(self.root, os.defpath), self.root, catalog=self.catalog)
        self.assertEqual(result["exit_code"], 1)
        self.assertEqual(result["diagnostics"]["status"], "incomplete")
        self.assertNotIn(secret, json.dumps(result["diagnostics"]))
        self.assertNotIn("injected_test.go", json.dumps(result["diagnostics"]))
        with self.assertRaises(TypeError):
            self.catalog["example.test/proof"]["tests"] = frozenset()

    def test_excessive_nesting_and_catalog_bounds_are_explicit(self):
        nested = "[" * 2000 + "0" + "]" * 2000
        result = diagnostics.failure_diagnostics(io.BytesIO(nested.encode() + b"\n"), self.catalog)
        self.assertEqual(result["status"], "incomplete")
        result = self.collect(self.record("output", Output="ZERB_ASSEMBLY_TRACE " + nested))
        self.assertEqual(result["status"], "incomplete")
        for name, value in (("MAX_CATALOG_BYTES", 1), ("MAX_CATALOG_FILES", 1), ("MAX_SOURCE_BYTES", 1)):
            with self.subTest(name=name), mock.patch.object(diagnostics, name, value):
                with self.assertRaises(ValueError):
                    diagnostics.source_catalog(self.sources)

    def test_checkpoint_flush_and_fsync_error_is_not_success(self):
        path = self.root / "journal"
        with path.open("w") as stream, mock.patch.object(cloud.os, "fsync", side_effect=OSError("fixture")):
            with self.assertRaises(OSError):
                cloud.diagnostic_checkpoint(stream, {"event": "check-start"})
        from test_cloud_conformance import valid_report
        report = valid_report()
        report["failure"] = "diagnostic-checkpoint-unavailable"
        self.assertEqual(cloud.gate_status(report), "failed")

    def test_unavailable_start_checkpoint_prevents_execution(self):
        with mock.patch.object(cloud.subprocess, "run") as run:
            with self.assertRaises(OSError):
                cloud.run_check("proof-race", ["unused"], self.repo, {}, self.root,
                                mock.Mock(side_effect=OSError("fixture")), self.catalog)
        run.assert_not_called()

    def test_timeout_preserves_known_failure_location_but_not_an_invented_exit(self):
        def interrupted(*args, **kwargs):
            records = [self.record("output", Output="fixture_test.go:3: private"), self.record("fail")]
            kwargs["stdout"].write(b"".join(json.dumps(row).encode() + b"\n" for row in records))
            raise subprocess.TimeoutExpired("fixture", 1)

        with mock.patch.object(cloud.subprocess, "run", side_effect=interrupted):
            result = cloud.run_check("proof-race", ["unused"], self.repo, {}, self.root, catalog=self.catalog)
        self.assertIsNone(result["exit_code"])
        self.assertEqual(result["status"], "failed")
        self.assertEqual(result["diagnostics"]["locations"][0]["line"], 3)

    def test_existing_journal_stops_before_checks_and_preserves_evidence(self):
        output = self.root / "report.json"
        journal = Path(str(output) + ".checks.jsonl")
        journal.write_text("existing retained evidence")
        with mock.patch.object(cloud, "collect") as collect:
            self.assertEqual(cloud.main(["--output", str(output)]), 1)
        collect.assert_not_called()
        self.assertEqual(journal.read_text(), "existing retained evidence")

    def test_new_journal_owner_mode_is_independent_of_umask(self):
        path = self.root / "restrictive-journal"
        prior = os.umask(0o777)
        try:
            fd = cloud.reserve_output(path, self.repo)
        finally:
            os.umask(prior)
        os.close(fd)
        self.assertEqual(path.stat().st_mode & 0o777, 0o600)


if __name__ == "__main__":
    unittest.main()
