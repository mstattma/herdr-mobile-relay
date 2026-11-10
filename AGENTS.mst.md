# Fork Instructions (mstattma/herdr-mobile-relay)

These rules apply in addition to `AGENTS.md`, which describes the upstream
project (`0cv/herdr-mobile-relay`). Where they conflict, this file wins for work
in this fork. This file lives only on the `mst` branch; never include it in a
branch meant for upstream.

## Branches

- `mst` is the integration branch: merge each upstream release tag while
  retaining our commits, without rewriting published history or force-pushing.
  Fork releases are tagged on `mst` as `vX.Y.Z+mst.N`.
- Each change meant for upstream also lives on its own topic branch based on
  `upstream/dev`, without fork-only commits (this file, version scheme, asset
  counter offsets).
- Fork-only commits are marked `[mst]` in the subject.

## Public repository: no private data

- This fork is public. Never commit captures from real sessions, real session
  IDs, repository or organization names, hostnames, tailnet names, paths of the
  owner's projects, tokens or pairing links. Tests use synthetic fixtures only.
- Private evaluation notes, real pane captures and deployment records stay in
  the private `/root/herdr-patches/notes/` repository and
  `/root/opencode-worktrees/mobile-eval/fixtures/`.

## Upstream contributions need explicit approval

- Never open, update or comment on upstream pull requests or issues, and never
  push to the `upstream` remote, without an explicit instruction from the owner
  for that specific change. Changes are tested in the owner's deployment for
  several days first.
- The `upstream` remote has its push URL disabled on purpose; do not re-enable it.
- When approved, follow upstream `AGENTS.md`: PRs from a topic branch into
  `dev`, tests, and a `CHANGELOG.md` entry.

## Versions and builds

- Version scheme `X.Y.Z+mst.N` (semver build metadata): `X.Y.Z` is the upstream
  release the build is based on, `N` counts our builds of it. Version comparison
  ignores the `+mst.N` part, so upstream releases are still announced.
- Lazily loaded web chunks are named by the `assets` counter in
  `frontend/build-versions.json` and served as immutable. Every fork build must
  use a counter no upstream build uses: `upstream assets * 1000 + N`. Reusing a
  number lets phones mix cached chunks from another build and crash
  (`effect_orphan`).
- Builds are local for now: Go and Bun at the versions pinned in CI, run inside
  memory-capped systemd units on the owner's host (see the host's global agent
  instructions). The deployment is installed from local release bundles, not
  through the relay's managed updater.
