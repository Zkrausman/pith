import copy
import io
from contextlib import redirect_stdout
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("publisher", Path(__file__).parents[1] / "scripts/publish_release.py")
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)


class PublicationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.directory = self.root / "dist"
        self.directory.mkdir()
        self.notes_directory = self.root / "docs" / "release-notes"
        self.notes_directory.mkdir(parents=True)
        self.notes_file = self.notes_directory / "v2.4.7.md"
        self.notes = "# Pith v2.4.7\n\nReviewed migration: café.\n\n"
        self.notes_file.write_bytes(self.notes.encode("utf-8"))
        self.root_patch = patch.object(publisher, "REPOSITORY_ROOT", self.root)
        self.root_patch.start()
        self.addCleanup(self.root_patch.stop)
        for name in publisher.ASSETS:
            (self.directory / name).write_bytes(b"synthetic-" + name.encode())
        import hashlib
        (self.directory / "checksums.txt").write_text("".join(
            hashlib.sha256((self.directory / name).read_bytes()).hexdigest() + "  " + name + "\n"
            for name in publisher.BINARIES))
        self.expected = publisher.local_assets(self.directory)
        self.remote = {"tag_name": "v2.4.7", "draft": True, "body": self.notes, "assets": [
            {"name": name, "state": "uploaded", **value} for name, value in self.expected.items()]}
        self.calls = []

    def run_gh(self, *args):
        self.calls.append(args)
        if args[:2] == ("release", "view"):
            return json.dumps({"databaseId": 123})
        if args[:2] == ("release", "edit"):
            self.remote["draft"] = False
        if args[0] == "api":
            # Real GitHub does not expose draft releases through /tags/{tag}.
            if args[1] != "repos/owner/repo/releases/123":
                raise subprocess.CalledProcessError(1, "gh", stderr="HTTP 404")
            return json.dumps(self.remote)
        return ""

    def test_publish_serially_then_verify_before_publication(self):
        publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
        self.assertEqual([c[3] for c in self.calls if c[:2] == ("release", "upload")],
                         [str(self.directory / n) for n in publisher.ASSETS])
        self.assertEqual([c[0] for c in self.calls], ["release"] * 8 + ["api", "release", "api"])
        self.assertTrue(all("--clobber" not in c for c in self.calls))
        self.assertEqual(self.calls[0], ("release", "create", "v2.4.7", "--repo",
                         "owner/repo", "--verify-tag", "--draft", "--title",
                         "v2.4.7", "--notes", self.notes))
        self.assertEqual(len(self.expected), 6)

    def test_invalid_release_id_never_publishes(self):
        for invalid in [None, "123", 0, -1, True]:
            self.calls = []
            def run(*args):
                if args[:2] == ("release", "view"):
                    self.calls.append(args)
                    return json.dumps({"databaseId": invalid})
                return self.run_gh(*args)
            with self.assertRaises(ValueError):
                publisher.publish("owner/repo", "v2.4.7", self.directory, run)
            self.assertFalse(any(c[:2] == ("release", "edit") for c in self.calls))

    def test_upload_failure_never_publishes_or_retries(self):
        def fail(*args):
            self.run_gh(*args)
            if args[:2] == ("release", "upload"):
                raise subprocess.CalledProcessError(1, "gh")
            return ""
        with self.assertRaises(subprocess.CalledProcessError):
            publisher.publish("owner/repo", "v2.4.7", self.directory, fail)
        self.assertEqual(len(self.calls), 2)
        self.assertTrue(self.remote["draft"])

    def test_remote_mismatches_never_publish(self):
        changes = [lambda r: r["assets"].pop(),
                   lambda r: r["assets"].append(copy.deepcopy(r["assets"][0])),
                   lambda r: r["assets"][0].update(digest="sha256:wrong"),
                   lambda r: r["assets"][0].update(size=0),
                   lambda r: r["assets"][0].update(state="starter"),
                   lambda r: r.update(tag_name="v0.0.0"),
                   lambda r: r.update(draft=False),
                   lambda r: r.update(body="generated notes"),
                   lambda r: r.update(body=None),
                   lambda r: r.pop("body")]
        original = copy.deepcopy(self.remote)
        for change in changes:
            with self.subTest(change=change):
                self.remote = copy.deepcopy(original)
                self.calls = []
                change(self.remote)
                with self.assertRaises(ValueError):
                    publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
                self.assertFalse(any(c[:2] == ("release", "edit") for c in self.calls))

    def test_local_mismatch_fails_before_network(self):
        (self.directory / publisher.BINARIES[0]).write_bytes(b"tampered")
        with self.assertRaises(ValueError):
            publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
        self.assertEqual(self.calls, [])

    def test_invalid_identifiers_fail_before_network(self):
        for repo, tag in [("--bad", "v2.4.7"), ("owner/repo", "../bad"), ("owner/repo", "v02.4.7"),
                          ("owner/repo", "v2.04.7"), ("owner/repo", "v2.4.07")]:
            with self.assertRaises(ValueError):
                publisher.publish(repo, tag, self.directory, self.run_gh)
        self.assertEqual(self.calls, [])

    def test_invalid_notes_fail_before_any_remote_call(self):
        invalid = [b"", b" \n", b"# Pith v2.4.7\n\n",
                   b"# Pith v2.4.8\nWrong version\n",
                   b"Intro\n# Pith v2.4.7\nBody\n",
                   b"# Pith v2.4.7\nInvalid \xff\n",
                   b"# Pith v2.4.7\nNUL \x00\n"]
        for content in invalid:
            with self.subTest(content=content):
                self.notes_file.write_bytes(content)
                with self.assertRaises((ValueError, UnicodeDecodeError)):
                    publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
                self.assertEqual(self.calls, [])

    def test_missing_nonregular_and_symlink_notes_fail_before_network(self):
        self.notes_file.unlink()
        with self.assertRaises(FileNotFoundError):
            publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
        self.notes_file.mkdir()
        with self.assertRaises(ValueError):
            publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
        self.notes_file.rmdir()
        target = self.root / "external.md"
        target.write_text(self.notes)
        self.notes_file.symlink_to(target)
        with self.assertRaises(ValueError):
            publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
        self.assertEqual(self.calls, [])

    def test_symlink_parent_escape_fails_before_network(self):
        for directory in (self.notes_directory, self.root / "docs"):
            with self.subTest(directory=directory):
                moved = self.root / "outside"
                directory.rename(moved)
                directory.symlink_to(moved, target_is_directory=True)
                try:
                    with self.assertRaises(ValueError):
                        publisher.publish("owner/repo", "v2.4.7", self.directory, self.run_gh)
                    self.assertEqual(self.calls, [])
                finally:
                    directory.unlink()
                    moved.rename(directory)

    def test_exact_reviewed_body_is_snapshot_not_reread(self):
        self.notes = "# Pith v2.4.7\r\n\r\nReviewed café.\r\n\r\n"
        self.notes_file.write_bytes(self.notes.encode("utf-8"))
        self.remote["body"] = self.notes
        def run(*args):
            if args[:2] == ("release", "create"):
                self.notes_file.write_text("changed after validation")
            return self.run_gh(*args)
        publisher.publish("owner/repo", "v2.4.7", self.directory, run)
        self.assertEqual(self.calls[0][-2:], ("--notes", self.notes))

    def test_post_public_notes_mismatch_reports_failure_without_retry(self):
        def run(*args):
            result = self.run_gh(*args)
            if args[:2] == ("release", "edit"):
                self.remote["body"] = self.notes.rstrip()
            return result
        output = io.StringIO()
        with redirect_stdout(output), self.assertRaisesRegex(ValueError, "notes"):
            publisher.publish("owner/repo", "v2.4.7", self.directory, run)
        self.assertFalse(self.remote["draft"])
        self.assertNotIn("Published verified release", output.getvalue())
        self.assertEqual(sum(c[:2] == ("release", "edit") for c in self.calls), 1)
        self.assertEqual(len(self.calls), 11)

    def test_existing_release_creation_failure_stops(self):
        def fail(*args):
            self.calls.append(args)
            raise subprocess.CalledProcessError(1, "gh")
        with self.assertRaises(subprocess.CalledProcessError):
            publisher.publish("owner/repo", "v2.4.7", self.directory, fail)
        self.assertEqual(len(self.calls), 1)


if __name__ == "__main__":
    unittest.main()

