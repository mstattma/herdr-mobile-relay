# Fork Builds

`mst` is based on upstream **v0.24.0**. Our release is **0.24.0+mst.1**.
Build metadata does not affect version ordering, so the Settings page still
announces newer upstream releases. Installing one replaces our patches;
there are no automatic installs. Builds are local for now, not GitHub releases.

## Changes

- OpenCode permission cards, including subagent requests and follow-up dialogs.
- Complete wrapped question text, question history, and empty-question controls.
- Readable app error stacks.
- Discovery of per-channel OpenCode databases (`opencode-<channel>.db`).
- Fork-only version metadata and a separate web asset revision range.

Changes are tested here before asking the owner to approve upstream PRs. No
upstream PR or issue is submitted automatically. See `AGENTS.mst.md`.

## Build

Use the toolchain versions in `.github/workflows/check.yml`. On a constrained
host, run these commands sequentially inside a resource-limited job and monitor
its memory. No dependency or global configuration changes are required.

```sh
bun install --frozen-lockfile --cwd frontend
make frontend-check
bun frontend/scripts/release.mjs
HERDR_RELEASE_TARGETS=linux/amd64 \
  sh scripts/package-release.sh 0.24.0+mst.1 "$(git rev-parse HEAD)" dist/release
```

`HERDR_RELEASE_TARGETS` can select a host target for a local build; omit it to
produce all four upstream targets. The archive includes the executable, web
bundle, wrappers, and a verified release manifest. `dist/release/checksums.txt`
records its SHA-256.

For a new build, update the manifest's `+mst.N` suffix and the fork label in the
README, and set `frontend/build-versions.json`'s `assets` to
`<upstream tag's assets> * 1000 + N` **before** building. Lazy chunks use that
number in their filenames and are served immutable: never ship different bytes
under a previously used name. Commit the generated `web/` bundle after
verification, then package from the clean commit and tag `vX.Y.Z+mst.N` on `mst`.

## Install

The fork is not published through the upstream plugin downloader. Link a local
checkout with `herdr plugin link /absolute/path/to/checkout`, extract its verified
archive into a new versioned release directory, and switch the existing
`herdr-mobile-relay/current` symlink to that directory. Keep the previous release
for rollback, keep device/configuration directories unchanged, and restart only
the relay. This is the same release-directory layout used by upstream's
installer, without downloading or claiming an upstream tag for fork binaries.

The packaged `version.json` distinguishes the core browser version (`version`)
from the full fork deployment identity (`release_version`), with the exact
source commit in `revision`. `herdr-mobile-relay version --json` reports the full
fork version.
