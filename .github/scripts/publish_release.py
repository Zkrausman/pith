"""Publish an immutable, complete release; leave failures as drafts, never retry/clobber."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

BINARIES = ("pith-darwin-arm64", "pith-linux-amd64", "pith-windows-amd64.exe")
ASSETS = BINARIES + ("checksums.txt", "checksums.txt.sig", "sbom.cdx.json")


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True)


def local_assets(directory):
    directory = Path(directory)
    if {p.name for p in directory.iterdir()} != set(ASSETS):
        raise ValueError("release directory must contain exactly the six expected assets")
    expected = {}
    for name in ASSETS:
        file = directory / name
        if file.is_symlink() or not file.is_file() or file.stat().st_size == 0:
            raise ValueError("release asset must be a nonempty regular file: " + name)
        expected[name] = {"size": file.stat().st_size,
                          "digest": "sha256:" + hashlib.sha256(file.read_bytes()).hexdigest()}
    manifest = {}
    for line in (directory / "checksums.txt").read_text().splitlines():
        match = re.fullmatch(r"([a-f0-9]{64})  (pith-[A-Za-z0-9.-]+)", line)
        if not match or match[2] in manifest:
            raise ValueError("invalid or duplicate checksum entry")
        manifest[match[2]] = "sha256:" + match[1]
    if manifest != {name: expected[name]["digest"] for name in BINARIES}:
        raise ValueError("binary checksums do not match manifest")
    return expected


def verify_remote(release, tag, expected, draft):
    if release.get("tag_name") != tag or release.get("draft") is not draft:
        raise ValueError("unexpected release identity/state")
    assets = release.get("assets", [])
    if len(assets) != len(expected) or {a.get("name") for a in assets} != set(expected):
        raise ValueError("remote release asset set is incomplete or unexpected")
    for asset in assets:
        if asset.get("state") != "uploaded" or any(
            asset.get(key) != value for key, value in expected[asset["name"]].items()
        ):
            raise ValueError("remote size/digest/state mismatch: " + asset["name"])


def publish(repository, tag, directory, run=gh):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("invalid repository")
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("invalid release tag")
    expected = local_assets(directory)
    # Fail if a release already exists. No tag recreation, replacement, or retry.
    run("release", "create", tag, "--repo", repository, "--verify-tag", "--draft",
        "--title", tag, "--generate-notes")
    for name in ASSETS:
        print("Uploading " + name, flush=True)
        run("release", "upload", tag, str(Path(directory) / name), "--repo", repository)
    # GitHub's by-tag REST endpoint excludes drafts. gh resolves drafts through
    # the release list; pin its numeric ID for both pre/post-publication checks.
    release_id = json.loads(run("release", "view", tag, "--repo", repository,
                                "--json", "databaseId"))["databaseId"]
    if type(release_id) is not int or release_id <= 0:
        raise ValueError("invalid release database ID")
    endpoint = f"repos/{repository}/releases/{release_id}"
    verify_remote(json.loads(run("api", endpoint)), tag, expected, True)
    run("release", "edit", tag, "--repo", repository, "--draft=false", "--latest")
    verify_remote(json.loads(run("api", endpoint)), tag, expected, False)
    print("Published verified release " + tag, flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("repository")
    parser.add_argument("tag")
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    publish(args.repository, args.tag, args.directory)
