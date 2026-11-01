## Why

Personal development context belongs beside a code checkout but often must not enter the shared code repository. Devdocs makes that context portable across machines through an independent private Git history, without a custom file-copy synchronization layer.

## What Changes

- Add a macOS/Linux `devdocs` executable using installed Git.
- Keep a real `devdocs/` nested checkout, backed by one independent UUID-named project branch in a configured private remote.
- Provide `config`, `init`, `list`, `attach`, `status`, `sync`, `git`, `link`, and `unlink` with explicit network access and ordinary Git recovery.
- Add explicit `init --adopt` migration for existing unmanaged documents without copying, moving, or overwriting them.
- Exclude personal paths locally, reject tracked/colliding paths, and apply synchronized link declarations only on explicit request.
- Preserve files and commits on failures; never force-push, auto-resolve conflicts, or execute downloaded content.
- Document privacy limits, ignored-file omissions, worktrees, and recovery.
- Publish tagged macOS/Linux binaries with checksums and a verified curl-to-shell installer.

## Capabilities

### New Capabilities

- `workspace-lifecycle`: global configuration, offline initialization, explicit attachment, project discovery, and stable identity.
- `git-sync`: project-scoped snapshots, history transfer, merge and publication, and recoverable failures.
- `repository-isolation`: effective local exclusions, index protection, linked worktree support, and safe filesystem boundaries.
- `path-links`: explicit file/directory symlinks, versioned declarations, machine-local ownership, collision refusal, and unlinking.
- `workspace-status`: offline publication, changes, ignored files, Git operation, safety, and link diagnostics.
- `release-distribution`: tagged release builds, verified installation, and post-release install checks.

### Modified Capabilities

None. This is a new project with no existing behavior to migrate.

## Impact

Creates the Go CLI and regression tests in this repository. Runtime dependencies are Git 2.39+ and a configured private remote; hosting-provider APIs, agent installers, background sync, encryption, multiple stores, Windows, and remote deletion are excluded. The product source is the agreed Obsidian `Projects/Devdocs.md` description; its A1–A17 scenarios are covered by these capability specs.
