# Releasing

Releases are tag-driven. The CHANGELOG is the source of truth for release notes; the workflow extracts the relevant section and feeds it to goreleaser.

## Cut a release

1. Edit `CHANGELOG.md`:
   - Rename the `## [Unreleased]` heading to `## [X.Y.Z] - YYYY-MM-DD`.
   - Add a fresh empty `## [Unreleased]` block above it.
2. Commit:
   ```
   git add CHANGELOG.md
   git commit -m "Release vX.Y.Z"
   git tag vX.Y.Z
   git push origin main vX.Y.Z
   ```

## What the workflow publishes

On `v*` tag push, `.github/workflows/release.yml` runs three jobs in parallel-ish:

- **build-image** — multi-arch (`linux/amd64`, `linux/arm64`) Docker image to `ghcr.io/<owner>/<repo>` with semver, branch, and `latest` tags.
- **release-binaries** — Linux/macOS/Windows × amd64/arm64 archives via goreleaser; release notes are extracted from `CHANGELOG.md` by `scripts/changelog-section.sh`.
- **publish-chart** — the Helm chart in `install/kubernetes/github-actions-cache-server` is repackaged with `Chart.yaml` `appVersion` and `version` synced to the release tag, then pushed to `oci://ghcr.io/<owner>/charts`.

## Commit messages

Plain English, imperative mood. No conventional-commits prefixes (`feat:`, `fix:` etc.) — they're not used to derive the changelog or anything else, so they're noise. Examples:

- `Add disk-pressure LRU eviction`
- `Fix off-by-one in block-id parser`
- `Rewrite README cache-redirect section`
- `Bump go-sql-driver/mysql to v1.10.0`

If you want a longer body, separate it from the subject with a blank line.

## Versioning

[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Pre-1.0:
- Bump minor (`0.X.0`) for new features or breaking changes.
- Bump patch (`0.Y.Z`) for bugfixes.

Once we hit 1.0, breaking changes bump major.
