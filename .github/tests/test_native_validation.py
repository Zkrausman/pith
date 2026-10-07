import importlib.util
import itertools
import json
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "native_validation.py"
spec = importlib.util.spec_from_file_location("native_validation", SCRIPT)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class NativeValidationTests(unittest.TestCase):
    def setUp(self):
        self.allowlist = json.loads(SCRIPT.with_name("native-doc-paths.json").read_text())
        self.event = {"before": "a" * 40, "after": "b" * 40,
                      "pull_request": {"base": {"sha": "a" * 40}, "head": {"sha": "b" * 40}}}

    def classify(self, raw, name="pull_request", ref="refs/pull/123/merge"):
        def diff(*args):
            self.assertEqual(args, ("diff", "--name-only", "--no-renames", "-z",
                                    "a" * 40, "b" * 40, "--"))
            return raw
        return gate.required(self.event, name, ref, self.allowlist, diff)[0]

    def test_exact_docs_only(self):
        raw = b"README.md\0docs/releases.md\0CHANGELOG.md\0"
        self.assertFalse(self.classify(raw))
        self.assertFalse(self.classify(raw, "push", "refs/heads/main"))

    def test_unknown_build_and_embedded_paths_require_validation(self):
        for path in ("main.go", "go.mod", "go.sum", ".github/workflows/test.yml",
                     ".github/scripts/native-doc-paths.json", "pkg/gui/static/README.md",
                     "pkg/gui/dashboard.html", "pkg/selfupdate/pith-release.pub",
                     "pkg/anomaly/emutls_stub_windows.c", "testdata/README.md",
                     "docs/new-unknown.md", "pith-parser-generator/SKILL.md", "unknown\nfile.md"):
            with self.subTest(path=path):
                self.assertTrue(self.classify(b"README.md\0" + path.encode() + b"\0"))

    def test_incomplete_and_invalid_comparisons(self):
        for raw in (b"", b"README.md", b"README.md\0\0", b"\xff\0"):
            self.assertTrue(self.classify(raw))
        for invalid in (None, "", "0" * 40, "main", "--help"):
            self.event["pull_request"]["head"]["sha"] = invalid
            self.assertTrue(self.classify(b"README.md\0"))
        def fail(*args):
            raise subprocess.CalledProcessError(128, "git")
        self.assertTrue(gate.required({}, "push", "refs/heads/main", self.allowlist, fail)[0])
        self.assertTrue(gate.required({"before": "a" * 40, "after": "b" * 40},
                                     "push", "refs/heads/main", self.allowlist, fail)[0])

    def test_invalid_allowlist_and_unknown_events(self):
        for paths in ([], {}, [None]):
            self.assertTrue(gate.required(self.event, "push", "refs/heads/main", paths)[0])
        for event, ref in (("workflow_dispatch", "refs/tags/v3.0.3"),
                           ("push", "refs/tags/v3.0.3"), ("push", "refs/heads/other")):
            self.assertTrue(self.classify(b"README.md\0", event, ref))

    def test_actual_git_rename_checks_both_sides(self):
        # Real Git diff, not a name-only mock: code -> docs must still build.
        with tempfile.TemporaryDirectory() as directory:
            def git(*args):
                return subprocess.run(["git", "-C", directory, *args], check=True,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout
            git("init")
            git("config", "user.name", "Fixture")
            git("config", "user.email", "fixture@example.invalid")
            path = Path(directory)
            (path / "code.go").write_text("fixture")
            git("add", ".")
            git("commit", "-m", "base")
            base = git("rev-parse", "HEAD").decode().strip()
            (path / "code.go").rename(path / "README.md")
            git("add", "-A")
            git("commit", "-m", "rename")
            head = git("rev-parse", "HEAD").decode().strip()
            event = {"before": base, "after": head}
            self.assertTrue(gate.required(event, "push", "refs/heads/main", self.allowlist, git)[0])
            # Reverse rename also cannot hide the new code path.
            self.assertTrue(gate.required({"before": head, "after": base}, "push",
                                          "refs/heads/main", self.allowlist, git)[0])

    def test_aggregate_exhaustive_results(self):
        statuses = ("success", "failure", "cancelled", "skipped", "")
        for tag, decision in itertools.product((False, True), ("true", "false", "")):
            for test, change, windows, build in itertools.product(statuses, repeat=4):
                results = dict(test=test, gate=change, windows=windows, build=build, required=decision)
                expected = (test == change == "success" and
                            windows == ("skipped" if tag else "success") and
                            decision in ("true", "false") and
                            (not tag or decision == "true") and
                            build == ("success" if decision == "true" else "skipped"))
                self.assertEqual(gate.accepted(results, "push" if tag else "pull_request",
                                              "refs/tags/v3.0.3" if tag else "refs/pull/1/merge"), expected,
                                 (tag, results))


class WorkflowContractTests(unittest.TestCase):
    def setUp(self):
        self.workflow = (SCRIPT.parents[1] / "workflows" / "test.yml").read_text()
        self.jobs = {}
        current = None
        for line in self.workflow.splitlines():
            if line.startswith("  ") and not line.startswith("   ") and line.endswith(":"):
                current = line.strip()[:-1]
                self.jobs[current] = []
            elif current:
                self.jobs[current].append(line)
        self.jobs = {name: "\n".join(lines) for name, lines in self.jobs.items()}

    def test_publication_is_push_tag_only_and_needs_validation(self):
        release = self.jobs["release"]
        self.assertIn("!cancelled()", release)
        self.assertIn("needs: [build, validation]", release)
        self.assertNotIn("always()", release)
        self.assertNotIn("workflow_dispatch:", self.workflow)
        self.assertNotIn("pull_request_target:", self.workflow)
        self.assertEqual(self.workflow.count("contents: write"), 1)
        self.assertEqual(self.workflow.count("secrets."), 2)

    def release_allowed(self, event, ref, build, validation, cancelled=False,
                        ancestors_succeeded=True):
        # Evaluate the actual conjunction's supported predicates, not a second
        # copy of the release condition. Unknown syntax fails the test closed.
        # This models documented status defaults, NOT the hosted scheduler.
        # https://docs.github.com/en/actions/reference/workflows-and-actions/expressions#status-check-functions
        expression = re.search(r"^    if: \$\{\{ (.+) \}\}$",
                               self.jobs["release"], re.MULTILINE).group(1)
        predicates = {
            "!cancelled()": not cancelled,
            "github.event_name == 'push'": event == "push",
            "startsWith(github.ref, 'refs/tags/v')": ref.startswith("refs/tags/v"),
            "needs.build.result == 'success'": build == "success",
            "needs.validation.result == 'success'": validation == "success",
        }
        terms = [term.strip() for term in expression.split("&&")]
        self.assertTrue(all(term in predicates for term in terms), expression)
        # Without a status function, implicit success gating rejects a skipped
        # ancestor even when the explicit event/ref condition would be true.
        if "!cancelled()" not in terms and not ancestors_succeeded:
            return False
        return all(predicates[term] for term in terms)

    def test_validated_tag_with_intentionally_skipped_windows_link_can_release(self):
        results = dict(test="success", gate="success", windows="skipped",
                       build="success", required="true")
        self.assertTrue(gate.accepted(results, "push", "refs/tags/v3.0.7"))
        self.assertTrue(self.release_allowed("push", "refs/tags/v3.0.7",
                                            "success", "success", ancestors_succeeded=False))

    def test_release_rejects_failed_missing_skipped_or_cancelled_dependencies(self):
        statuses = ("success", "failure", "cancelled", "skipped", "", None)
        for build, validation, cancelled in itertools.product(statuses, statuses, (False, True)):
            with self.subTest(build=build, validation=validation, cancelled=cancelled):
                self.assertEqual(self.release_allowed("push", "refs/tags/v3.0.7", build,
                                                      validation, cancelled),
                                 not cancelled and build == validation == "success")

    def test_release_rejects_non_tag_and_non_push_events(self):
        for event, ref in itertools.product(
                ("push", "pull_request", "pull_request_target", "workflow_dispatch", ""),
                ("refs/tags/v3.0.7", "refs/heads/main", "refs/heads/v3.0.7",
                 "refs/pull/1/merge", "refs/tags/other", "")):
            with self.subTest(event=event, ref=ref):
                self.assertEqual(self.release_allowed(event, ref, "success", "success"),
                                 event == "push" and ref == "refs/tags/v3.0.7")

    def test_artifact_validation_precedes_upload_and_publish(self):
        build = self.jobs["build"]
        self.assertLess(build.index("Build binary"), build.index("PITH_TEST_BINARY:"))
        self.assertLess(build.index("PITH_TEST_BINARY:"), build.index("actions/upload-artifact@"))
        self.assertIn("go test -count=1 -v . -run", build)
        release = self.jobs["release"]
        self.assertLess(release.index("cosign sign-blob"), release.index("PITH_TEST_RELEASE_DIST:"))
        self.assertLess(release.index("PITH_TEST_RELEASE_DIST:"), release.index("actions/upload-artifact@"))
        self.assertLess(release.index("PITH_TEST_RELEASE_DIST:"), release.index("publish_release.py"))
        self.assertIn("go test -count=1 -v ./pkg/selfupdate -run '^TestReleaseArtifactSignature$'", release)

    def test_aggregate_and_unfiltered_source_checks(self):
        self.assertIn("if: ${{ always() }}", self.jobs["validation"])
        self.assertIn("needs: [test, native-validation-gate, windows-link-check, build]", self.jobs["validation"])
        self.assertNotIn("if:", self.jobs["test"])
        self.assertNotIn("paths-ignore:", self.workflow)
        self.assertNotIn("paths:", self.workflow)
        self.assertNotIn("continue-on-error:", self.workflow)
        self.assertIn("fetch-depth: 0", self.jobs["native-validation-gate"])


if __name__ == "__main__":
    unittest.main()
