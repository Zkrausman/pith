"""Fail-closed native package gate and aggregate, using only Git and stdlib."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

SHA = re.compile(r"[0-9a-f]{40}\Z")


def git(*args):
    return subprocess.run(["git", *args], check=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE).stdout


def required(event, event_name, ref, allowlist, run_git=git):
    """Only a complete, nonempty comparison of exact known docs can skip."""
    if event_name == "push" and ref.startswith("refs/tags/v"):
        return True, "release tag"
    try:
        if event_name == "pull_request":
            base = event["pull_request"]["base"]["sha"]
            head = event["pull_request"]["head"]["sha"]
        elif event_name == "push" and ref == "refs/heads/main":
            base, head = event["before"], event["after"]
        else:
            return True, "unrecognized event/ref"
        if any(not isinstance(sha, str) or not SHA.fullmatch(sha) or sha == "0" * 40
               for sha in (base, head)):
            return True, "missing or invalid comparison SHA"
        if not isinstance(allowlist, list) or not allowlist or any(
                not isinstance(path, str) or not path or "\x00" in path for path in allowlist):
            return True, "invalid documentation allowlist"
        # --no-renames includes BOTH old and new paths as delete/add. NUL
        # framing preserves spaces/newlines and fails closed on undecodable paths.
        raw = run_git("diff", "--name-only", "--no-renames", "-z", base, head, "--")
        if not raw or not raw.endswith(b"\0"):
            return True, "empty or incomplete comparison"
        paths = raw[:-1].decode("utf-8", errors="strict").split("\0")
        if not all(path and path in set(allowlist) for path in paths):
            return True, "non-allowlisted path"
        return False, f"all {len(paths)} changed paths explicitly documentation-only"
    except (KeyError, TypeError, ValueError, UnicodeError, OSError, subprocess.SubprocessError):
        return True, "comparison unavailable"


def accepted(results, event_name, ref):
    """Missing, cancelled, failed, and unexpectedly skipped jobs never pass."""
    tag = event_name == "push" and ref.startswith("refs/tags/v")
    if results.get("test") != "success" or results.get("gate") != "success":
        return False
    if results.get("windows") != ("skipped" if tag else "success"):
        return False
    decision = results.get("required")
    if decision not in ("true", "false") or (tag and decision != "true"):
        return False
    return results.get("build") == ("success" if decision == "true" else "skipped")


def main():
    if sys.argv[1:] == ["aggregate"]:
        results = {key: os.environ.get("RESULT_" + key.upper(), "")
                   for key in ("test", "gate", "windows", "required", "build")}
        print(json.dumps(results, sort_keys=True))
        return 0 if accepted(results, os.environ.get("GITHUB_EVENT_NAME", ""),
                             os.environ.get("GITHUB_REF", "")) else 1
    try:
        event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
        allowlist = json.loads(Path(__file__).with_name("native-doc-paths.json").read_text())
        needed, reason = required(event, os.environ.get("GITHUB_EVENT_NAME", ""),
                                  os.environ.get("GITHUB_REF", ""), allowlist)
    except (KeyError, ValueError, OSError):
        needed, reason = True, "event/allowlist unavailable"
    value = str(needed).lower()
    print(f"Native package validation required={value}: {reason}")
    # A failure to record the output fails the gate job; the aggregate rejects it.
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        output.write(f"required={value}\n")
    with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as summary:
        summary.write(f"Native package validation: **{value}** ({reason}).\n")
        summary.write(f"Effective checkout SHA: `{git('rev-parse', 'HEAD').decode().strip()}`\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
