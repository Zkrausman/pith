# Verified release publication

The maintainer runbook is `docs/releases.md`. Implementation sources are
`.github/workflows/test.yml`, `.github/scripts/publish_release.py`,
`pkg/selfupdate/selfupdate.go`, and `pkg/selfupdate/signature.go`.

## Authority and immutable failures

`AGENTS.md` separates merge from tag, publication, and installation authority.
Obtain explicit owner authorization for each; tag-triggered publication is
automatic, so publication authorization must precede the tag push. Releases
come from human-merged, validated commits and use a unique semantic version
shared by the tag, binary, and changelog. Version agreement is a maintainer
requirement, not an asserted CI check. Documentation/test-only changes are
version-bump exempt.

Preserve failed tags and drafts. Never move/reuse a failed release tag, rerun
publication, clobber/replace assets, or manually publish a failed draft under
this process. Corrections require a fresh version unless the owner explicitly
changes policy; diagnosis is not recovery authority.

## Integrity boundaries

Tests precede native Linux amd64, Windows amd64, and macOS arm64 builds; all
builds gate the release job. The six assets are `pith-linux-amd64`,
`pith-windows-amd64.exe`, `pith-darwin-arm64`, `checksums.txt`,
`checksums.txt.sig`, and `sbom.cdx.json`. The manifest covers only the binaries;
its signature authenticates the manifest, not the SBOM. CI retains the complete
set as `signed-release-<tag>` for 30 days before publication.

The publisher validates the exact local set and binary manifest, creates a new
draft, uploads serially without retry/clobber, resolves a positive numeric
release ID, and checks tag/state plus exact remote names/count/uploaded
states/sizes/SHA-256 metadata before publication. It repeats verification by the
same numeric ID after making the release public/latest. It compares metadata
for all six files, rather than re-downloading them or verifying the signature.
The separately authorized updater instead verifies the signed manifest with
its embedded key and the selected platform binary checksum before replacement;
it requires unique required assets and does not verify the SBOM/full set.

## Operational diagnosis

A draft's by-tag REST lookup can return 404. Resolve its numeric ID through
release listing/view and inspect the numeric-ID endpoint; failed lookup means
unknown state, not proof of absence. Record workflow result independently from
release state: failure can leave no release, a partial/complete draft, or an
already-public release if the public-state edit occurred before failure.

Separate build/preparation, upload/service, verification, and installation
failures. Retain sanitized run/step, tag/commit, release ID/state, expected versus
observed asset metadata, error, retained artifact identity, and updater
platform/version/result. Exclude credentials, secrets, personal data, and
owner-local paths. Fail-closed integrity checks do not eliminate service outages
or authorize manual repair, bypass, or installation.
