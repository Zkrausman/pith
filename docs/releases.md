# Verified release publication and failure triage

Maintainer runbook for the current [CI workflow](../.github/workflows/test.yml),
[publisher](../.github/scripts/publish_release.py), and
[signed-checksum updater](../pkg/selfupdate/selfupdate.go).
This describes the existing pipeline, not permission to operate it.

## Authority and preflight

Follow [AGENTS.md](../AGENTS.md): owner authorization for **merge, tag,
publication, and installation is separate**. Merge approval authorizes none of
the later actions. Obtain explicit tag and publication authorization **before
pushing a tag**: the tag-triggered workflow publishes automatically. Publish
only from a human-merged commit after applicable validation succeeds.

- Choose a new, unique semantic version. Reconcile with current main and existing
  tags; an unmerged branch does not reserve a version.
- Require agreement between the `vMAJOR.MINOR.PATCH` tag, binary-reported version
  in [main.go](../main.go), and [CHANGELOG.md](../CHANGELOG.md) heading on that
  commit. Native package smoke now enforces exact binary/source/tag agreement and
  the matching changelog heading on tag pushes.
- Documentation-only and test-only PRs are version-bump exempt; record that
  decision in the PR description. Shipped-code changes follow AGENTS versioning.
- Never move or reuse a release tag, even after workflow failure. Preserve failed
  tags and drafts; do not rerun publication, retry/clobber assets, replace them
  manually, or manually publish a failed draft. A corrected release requires a
  fresh version unless the owner explicitly changes policy. This runbook grants
  no exception or manual recovery authority.

## Build and retained release set

On a release tag, the test job runs Go tests and Python publication unit tests
before the native build matrix. DuckDB/CGo requires native compilation:

| Platform | Runner | Binary asset |
| --- | --- | --- |
| Linux amd64 | `ubuntu-latest` | `pith-linux-amd64` |
| Windows amd64 | `windows-latest` | `pith-windows-amd64.exe` |
| macOS arm64 | `macos-14` | `pith-darwin-arm64` |

All matrix builds and the `validation` aggregate must succeed before the release
job. The release job requires **both a push event and a v-tag ref**; PR/main
validation does not sign or publish. There is no manual publication route.
Non-tag CI also keeps its existing native Windows linking check.

### Pre-tag native validation

The ordinary Linux source suite/vet/Python and Windows focused checks are
unfiltered. Extra native package builds run for PR/main changes unless **every**
old/new changed path belongs to the exact reviewed documentation list in
[`.github/scripts/native-doc-paths.json`](../.github/scripts/native-doc-paths.json).
The classifier uses full PR base/head or push before/after commits and a
NUL-delimited Git diff with rename detection disabled, so both rename sides
are checked. Unknown paths (including new Markdown), embedded assets, build
inputs, malformed/empty comparisons, or unavailable diffs require validation.
Release tags always require it. The gate records the effective checkout SHA.
No top-level path filters hide the existing CI checks.

Each native matrix job checks host and target OS/architecture (plus `uname -m`
and `sw_vers` on macOS), runs the anomaly and focused runner regressions, and
builds the distribution binary with `go build -trimpath ... .`. It then points
`TestCLIExitStatus` at that exact `dist` binary via `PITH_TEST_BINARY`; a missing,
empty, or nonregular requested path fails instead of recompiling. The smoke log
records its SHA-256. The harness isolates HOME/USERPROFILE/PITH_STORAGE, stamps
the update-check time, and uses a deterministic local helper without providers.
It checks exact version/help, normal/raw statuses (including Windows 301), long
combined raw output, UTF-8, final newlines, and existing failure cases before
uploading the artifact. `-count=1` prevents a cached test from satisfying this
artifact check. These are focused native checks, not a full Windows/macOS suite.

The always-running `validation` aggregate rejects failed, cancelled, missing,
or unexpectedly skipped jobs and invalid gate outputs. The only accepted build
skip is a successful documentation-only classification; the only accepted
Windows-link skip is a tag push, whose native package job runs the same relevant
coverage. No claim is made that repository branch protections require this new
check; maintainers must inspect its exact-head result before merging.

These checks close previously **missing** package/macOS validation, not tests
known to have failed. Green source checks alone remain different evidence from
successful smoke of the produced artifact.

The release job collects the binaries, generates a CycloneDX dependency SBOM,
and signs the binary checksum manifest with Cosign. The exact six-file set is:

- The three binaries above.
- `checksums.txt`: SHA-256 checksums for **only those three binaries**.
- `checksums.txt.sig`: signature authenticating the bytes of `checksums.txt`.
- `sbom.cdx.json`: CycloneDX dependency inventory.

After signing, `TestReleaseArtifactSignature` consumes the actual `dist` folder
with `PITH_TEST_RELEASE_DIST` and `-count=1`. It verifies the generated manifest
with the updater's existing embedded production public key, then verifies all
three binary hashes. A missing/empty requested input, invalid signature, wrong
signing key, missing binary, or hash mismatch stops the release before artifact
retention and publication. Ordinary CI tests missing/malformed/wrong-key
rejection; it does not claim to validate a future signed candidate. The actual
positive signature receipt is only available after the authorized tag job signs.
No new key, secret, verifier dependency, or permission is introduced.

Before invoking the publisher, CI retains the complete verified set as the Actions
artifact `signed-release-<tag>` for **30 days**. Retain its identity for diagnosis;
it is evidence, not permission to install or manually repair a release. The
manifest signature does not directly cover the SBOM or every release asset.

## Draft-to-public sequence

The publisher performs these steps in order:

1. Validate repository/tag syntax and exactly six nonempty regular local files
   (no symlinks). Compute each file's size and SHA-256; require a valid,
   duplicate-free manifest matching exactly the three binary digests.
2. Create a **new draft** with the existing tag verified and generated notes.
   Creation failure stops the process; it does not adopt an existing release.
3. Upload one asset at a time: macOS binary, Linux binary, Windows binary,
   checksums, signature, SBOM. There is no publisher retry loop or `--clobber`.
4. Resolve the draft through `gh release view` to a positive integer
   `databaseId`. Read `repos/{owner}/{repo}/releases/{release_id}` through REST.
5. Require the expected tag and draft state, exact asset names and count, every
   asset in `uploaded` state, and sizes and `sha256:` digest metadata matching
   the local files for **all six assets**. This compares GitHub's metadata; the
   publisher does not re-download assets; the earlier workflow test has already
   cryptographically verified the local manifest signature.
6. Set the release public and latest, then repeat the same verification through
   the same numeric-ID endpoint, now requiring public state. Only then report
   `Published verified release <tag>`.

These checks fail closed before publication on detected mismatches. They do
**not** prevent GitHub or other service outages, and publication is not atomic
with the final check: a failed workflow can already have a public release.

## Failure triage: observe, preserve, escalate

First identify the failing job/step and record the workflow result **separately
from the observed release state**. Stop publication attempts and retain evidence
for the owner; do not repair assets or bypass verification.

**Draft lookup caveat:** GitHub's releases-by-tag REST endpoint
(`repos/{owner}/{repo}/releases/tags/{tag}`) can return **404 for a draft**.
That alone does not prove absence. With authorized repository access, inspect
release listings or `gh release view` for the numeric `databaseId`, then inspect
the numeric-ID endpoint used above. If lookup fails, record state as unknown,
not absent. A tag's existence also does not prove a release exists.

| Category | Diagnose without changing release state |
| --- | --- |
| Build/preparation | Inspect test/native build logs, including DuckDB/CGo linking. If builds passed, distinguish artifact collection, SBOM generation, signing, or artifact-retention failures. Publication has not been reached; confirm whether any release exists rather than assuming. Do not expose signing secrets. |
| Upload/service | Identify draft creation, the last attempted upload, ID lookup, API read, or public-state edit as the failed operation. Preserve HTTP/service errors and observed state: there may be no draft, an empty/partial draft, a complete draft, or a public release if the edit took effect before an error. A complete upload alone does not prove verification passed. Do not rerun or clobber. |
| Verification | Compare the retained local set with observed tag/state, names/count, upload states, sizes, and digests. Local validation fails before network calls; pre-publication remote mismatch stops publication. A post-publication mismatch or API failure can leave the release public; do not describe every failure as draft-only or install it as a verified success. Escalate without manual publication recovery. |
| Installation | Treat as a separately authorized operation, not evidence that publication failed. Distinguish unsupported/missing platform assets, duplicate required assets, network/download failures, signature/checksum rejection, and filesystem replacement/rollback errors. Record the updater result and observed version; never substitute an unchecked binary or bypass signature/asset checks. |

### Evidence to retain

Use sanitized diagnostics, not raw environment dumps:

- Workflow/run reference, failed job/step, tag, and source commit.
- Numeric release ID if known, observed draft/public/unknown state, and when observed.
- Expected and observed asset names, counts, upload states, sizes, and SHA-256 digests.
- Relevant error/status output and retained `signed-release-<tag>` artifact identity
  (or the step that prevented its creation); preserve permitted evidence before expiry.
- For installation: platform/architecture, current and target versions, updater
  outcome, and observed version after an authorized attempt.

Exclude credentials, secret values, personal data, and owner-local machine paths.

## Separately authorized installation

The updater selects a newer semantic release and the runtime platform binary.
It requires unique binary, `checksums.txt`, and `checksums.txt.sig` assets. It
[verifies the manifest signature](../pkg/selfupdate/signature.go) using the
embedded release public key, requires a unique checksum for the selected binary,
and verifies the downloaded binary's SHA-256 before replacing the executable.
Missing, duplicate, invalid, or mismatched required integrity data refuses an
unverified update. Public update requests do not acquire local credentials.

Updater integrity checks are distinct from the publisher's six-asset checks:
the updater does not verify the SBOM or the entire release set. Network and
filesystem failures remain possible even for a correctly published release.
Neither a successful workflow nor possession of the retained artifact grants
installation authorization.


## First public v3 notes

Before authorizing the first v3 tag, review cumulative notes from the last public
v2.4.8 release, not just the latest patch. Generated PR-title notes are insufficient
for this major migration. Include the removal of the Thneed parser/exported Go
type and Thneed diagnostic enrichment (without claiming a replacement retrieval
engine), preserved normal child exit statuses, and explicit raw output semantics:
all captured stdout followed by stderr, without parsing or truncation, not
independent destinations or temporal interleaving. Include any subsequently
accepted patch from the final source/changelog. External diagnostics still
require explicit permission and use Gemini; no offline diagnostic claim follows
from removing Thneed. The updater can offer newer major versions.

This validation-only change does not supply tag/publication authorization,
change the production version, or claim a signed/public v3 release exists.
