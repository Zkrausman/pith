import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("publisher", Path(__file__).parents[1] / "scripts/publish_release.py")
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)


class PublicationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        for name in publisher.ASSETS:
            (self.directory / name).write_bytes(b"synthetic-" + name.encode())
        import hashlib
        (self.directory / "checksums.txt").write_text("".join(
            hashlib.sha256((self.directory / name).read_bytes()).hexdigest() + "  " + name + "\n"
            for name in publisher.BINARIES))
        self.expected = publisher.local_assets(self.directory)
        self.remote = {"tag_name": "v2.4.7", "draft": True, "assets": [
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
                   lambda r: r.update(draft=False)]
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
        for repo, tag in [("--bad", "v2.4.7"), ("owner/repo", "../bad")]:
            with self.assertRaises(ValueError):
                publisher.publish(repo, tag, self.directory, self.run_gh)
        self.assertEqual(self.calls, [])

    def test_existing_release_creation_failure_stops(self):
        def fail(*args):
            self.calls.append(args)
            raise subprocess.CalledProcessError(1, "gh")
        with self.assertRaises(subprocess.CalledProcessError):
            publisher.publish("owner/repo", "v2.4.7", self.directory, fail)
        self.assertEqual(len(self.calls), 1)


if __name__ == "__main__":
    unittest.main()
