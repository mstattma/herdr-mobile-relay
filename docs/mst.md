# Fork Builds

`mst` is based on upstream **v0.24.0**. Our release is **0.24.0+mst.5**.
Build metadata does not affect version ordering, so the Settings page still
announces newer upstream releases. Installing one replaces our patches;
there are no automatic installs. Builds are local for now, not GitHub releases.

## Changes

- OpenCode permission cards, including subagent requests and follow-up dialogs.
- Complete wrapped question text, question history, and empty-question controls.
- Readable app error stacks.
- Discovery of per-channel OpenCode databases (`opencode-<channel>.db`).
- Fork-only version metadata and a separate web asset revision range.
- Named sessions and abbreviated paths in the overview; session details with
  copyable native IDs, OpenCode todos, and optional locally exported status.
- A sessionless OpenCode composer, guarded default-Tab agent switching, and a
  device-local maximum prompt-line setting.
- Fixed-grid OpenCode terminals without unsafe TTY-only resizing. Conversation
  remains the mobile-friendly transcript; Terminal shows the current grid and
  supports horizontal scrolling rather than reflowing sidebar columns.

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
  sh scripts/package-release.sh 0.24.0+mst.5 "$(git rev-parse HEAD)" dist/release
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

## Foreground Startup

Linking the plugin does not install an automatic relay service. To start the
configured foreground transport in its own tab:

```sh
herdr plugin pane open --plugin herdr-mobile-relay.events \
  --entrypoint quick-start --placement tab --focus
```

Skip the optional speech-voice download with `n` if unwanted. Keep this pane
open; Ctrl-C stops the relay. Do not open another copy while it is running.
For status, use `herdr plugin action invoke status --plugin herdr-mobile-relay.events`.
Disconnecting a Herdr client does not itself stop its server or panes, but a
host/WSL shutdown does. Background-service setup is a separate operation;
the plugin's presence alone is not a guarantee of unattended startup.

## Optional Session Status

The public relay does not require a patched OpenCode or a subscription router.
Session titles, IDs, todos and channel databases use native OpenCode metadata;
the ID comes from Herdr's standard agent-session identity, not a custom sidebar.
Terminal question/permission and agent-cycle adapters remain version/layout
specific and refuse unfamiliar screens rather than guessing key presses.

Subscription observations are the only extension-specific part of this
deployment. They are read explicitly through an optional host exporter, not
transmitted by or scraped from the desktop sidebar. The exporter may reuse the
sidebar's presentation logic without depending on its visibility. No private
router implementation, store path or credentials are built into the public relay.

`HERDR_SESSION_DETAILS_COMMAND` is a JSON argv array configured by the host
administrator, not a command supplied by the phone. The relay invokes it only
for an authenticated session-details read and sends `{"session_id":"..."}` on
stdin. It expects `{"rows":[],"observed_at":0}` with optional rows containing
only display strings `label`, `provider`, `status`, `detail`, `weeklyReset`,
`limits`, `paid`, and a boolean `current`. The timestamp is milliseconds since
the Unix epoch and should describe the observation, not a new provider poll.

The child receives a minimal environment. Use explicit argv (for example an
`env` wrapper) for any required data root or timezone. Keep exporters private
when they integrate private extensions; never publish real status snapshots.
Exporters must be read-only, must not refresh credentials or make network
requests, and must never include tokens or raw provider errors in display
fields. Output is bounded and unknown fields are discarded by the relay.

Leave the setting unset on stock installations: no provider process is launched
and the subscription payload is omitted. A configured exporter that is missing,
fails, times out, or returns invalid output only makes subscriptions unavailable;
session metadata and todos still return normally. The drawer reports that the
optional observations are unavailable. Missing todo tables likewise make only
todos unavailable, without preventing the session-details view from opening.

The OpenCode Switch agent action uses the default Tab binding at a fresh, empty
native prompt. It cycles primary agents; it does not claim to select a named
mode and does not support custom bindings or operation inside dialogs. It also
works while a request is running: OpenCode changes the selection for the next
submission without interrupting the already-running request.
