## Purpose

Keep private context outside the code repository's tracked content while making safety failures explicit rather than relying on bypassable hooks.

## ADDED Requirements

### Requirement: Effective local exclusions without tracked changes
Before exposing docs or links, the CLI MUST append escaped root-anchored local exclusion entries while retaining existing entries and verify their effective behavior. It MUST discover Git's actual common/exclusion paths, including linked worktrees, and MUST NOT assume `.git` is a directory. Init, attach, link, sync, and status MUST reject tracked managed paths or ineffective exclusions with actionable paths. It MUST NOT modify the code index, tracked files, HEAD, branches, remotes, shared `.gitignore`, or hooks.

#### Scenario: Normal isolation (A1, A10)
- **WHEN** a docs checkout or explicit link is installed
- **THEN** the code repository remains clean and its index and HEAD are unchanged, without a submodule or gitlink

#### Scenario: Tracked or negated path (A11, A12)
- **WHEN** a managed path is already indexed or project rules override the local exclusion
- **THEN** unsafe operations are refused without untracking files or editing the project's ignore rules

### Requirement: Filesystem and downloaded-content boundaries
The CLI MUST reject absolute, traversal, reserved `.git`/docs/internal-metadata paths, overlapping destinations, and parent-symlink escapes. Sources MUST resolve inside the docs tree; destinations MUST remain inside the code checkout outside docs and Git metadata. Remote declarations MUST NOT execute scripts or install hooks, and Devdocs MUST NOT collect external symlink contents. Remote URLs and filenames MUST be passed as process arguments, never interpolated into shell commands.

#### Scenario: Malicious link declaration (A11)
- **WHEN** a source/destination attempts traversal, reserved-path access, or symlink escape
- **THEN** the declaration is rejected without writing or deleting outside the allowed roots

### Requirement: Honest privacy and safety limits
Setup and usage documentation MUST explain private/authorized remote prerequisites, no secrets/encryption guarantee, ignored-file omission, persistence of deleted content in Git history, bypassable exclusions, and destructive clean risks. Normal diagnostics MUST NOT print document contents or credential-bearing remote URLs. The tool MUST NOT claim hosting-provider privacy verification.

#### Scenario: New workspace warning
- **WHEN** a user configures or initializes private context storage
- **THEN** the CLI states the authorized-private-storage/no-secrets requirement without claiming access-control enforcement
