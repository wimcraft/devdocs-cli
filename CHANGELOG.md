# Changelog

## 0.1.0

### Added

- `install.sh` for installing a checksum-verified release binary, and release builds for macOS and Linux on amd64 and arm64.
- A Go CLI for independent private development-context workspaces with `config`, `init`, `list`, `attach`, `status`, `sync`, `link`, and `unlink`.
- Explicit `devdocs init --adopt` migration for existing unmanaged documents, with in-place snapshots, case-insensitive template preservation, normal ignore rules, and rollback of only generated entries on failure.
- `devdocs git <args...>` for direct Git access in the docs checkout, with native streams/exit codes and metadata-independent recovery; recovery messages now use this command.
- Real nested docs repositories with stable UUID identities and one independently fetched project branch per private remote project.
- Explicit Git snapshots, ordinary merge/conflict recovery, publication confirmation, and retained local commits on network or push failures.
- Local repository exclusions, linked-worktree support, per-workspace process locks, and shared exclusion-update locking.
- Explicit relative path links with versioned declarations, local ownership, collision refusal, rollback, and non-destructive unlinking.
- Offline diagnostics for publication, staged/unstaged/untracked/ignored paths, pending Git operations, and link health.
- OpenSpec proposal, design, six capability deltas, completed implementation tasks, and verification evidence.

### Safety

- Reject tracked managed paths, ineffective exclusions, path traversal/symlink escapes, symlinked metadata/exclusion files, and mismatched project/branch/remote configuration.
- Inspect all configured fetch/push URLs and refspecs rather than allowing an additional unpinned remote to receive private history.
- Preserve existing and prior local exclusion entries, including sibling-worktree links.
- Parse Git status as NUL-delimited records so an unstaged first pathname is not truncated or incorrectly reported as staged.
- Keep credential-bearing URL userinfo out of Git diagnostics and invoke editors without shell evaluation.
- Refuse to sync a nested Git repository, which would otherwise publish as an empty directory while reporting success.
- Require private, authorized storage; provide no secrets-vault, encryption, or hosting-visibility guarantee.
