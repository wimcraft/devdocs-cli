## Purpose

Expose selected private documents at conventional code-project paths without copying content, replacing existing project files, or trusting synchronized setup data automatically.

## ADDED Requirements

### Requirement: Explicit versioned file and directory links
`link <source> <destination>` MUST create a relative symlink from a code-root-relative destination to an existing docs-relative file or directory, record the mapping in versioned metadata, and keep link ownership machine-local. Correct repeated application MUST be idempotent. `link --apply` MUST display and explicitly apply synchronized declarations, never run on attach/sync automatically, preflight the full set, and roll back only links created by a failed invocation.

#### Scenario: File and nested directory mappings (A10)
- **WHEN** a user links `AGENTS.md` or `skills` to `.claude/skills` and later attaches elsewhere
- **THEN** original files remain in docs and the second checkout creates links only after explicit apply

#### Scenario: Existing path collision (A11)
- **WHEN** a target is tracked or is an existing unmanaged file, directory, or different symlink
- **THEN** link refuses without replacing, adopting, or merging the target

### Requirement: Non-destructive unlink and stale ownership
`unlink <destination>` MUST delete only a locally owned symlink whose target still matches the ownership record and remove its versioned mapping when present. It MUST preserve source content, changed/unowned destinations, and safety exclusions; retained exclusions MUST be reported. Remote removal of a declaration MUST NOT delete local paths automatically. A stale locally owned link MUST remain explicitly removable even if its declaration was removed remotely.

#### Scenario: Changed destination or removed declaration (A16)
- **WHEN** a local link was manually replaced or its versioned declaration disappeared on another replica
- **THEN** status distinguishes changed/stale ownership, sync does not delete it, and explicit unlink removes only an unchanged locally owned symlink
