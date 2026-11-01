## Purpose

Create and recover independent private documentation workspaces without changing the shared code repository or requiring unrelated project histories.

## ADDED Requirements

### Requirement: Global configuration
The CLI MUST support `config`, `config --remote <git-url>`, and `config --edit`, using `$XDG_CONFIG_HOME/devdocs/config.json` or `~/.config/devdocs/config.json`. It MUST validate the configuration and use `$VISUAL` then `$EDITOR` for editing without evaluating shell expressions. Git authentication MUST remain delegated to Git; configuration and diagnostics MUST NOT store or expose embedded remote credentials. Changing the default MUST NOT retarget an existing workspace.

#### Scenario: Configure a future workspace
- **WHEN** the user sets a valid SSH, HTTPS, or local Git transport remote
- **THEN** subsequent init and attach use it, while existing workspaces retain their pinned remote

#### Scenario: Invalid configuration or editor
- **WHEN** configuration is malformed, a URL contains embedded credentials, or no editor is configured for editing
- **THEN** the command fails clearly without silently substituting a remote or executing configuration as code

### Requirement: Offline initialization and stable identity
`init [--adopt] [--name <name>]` MUST create a real nested docs Git repository, UUID identity, and independent `devdocs/<slug>-<uuid>` branch. New workspaces include `.devdocs.json`, `README.md`, and `FYI.md`; explicit adoption preserves existing templates as defined below. Initialization MUST work offline with configured remote and Git author identity, never invent identity, and never inherit another project's history. Required ignored generated files MUST cause a path-specific failure rather than force-add. Successful reinitialization MUST be idempotent.

#### Scenario: New offline project (A1, A2)
- **WHEN** init runs offline from a code repository or descendant with usable configuration and identity
- **THEN** it creates a locally committed unpublished workspace with a stable identity, preserving the source index, tracked files, and HEAD

#### Scenario: Repeat initialization (A2)
- **WHEN** init runs again in the same valid workspace
- **THEN** identity and commit count remain unchanged

#### Scenario: Global ignore hides a required template (A13)
- **WHEN** a required initial file is effectively ignored
- **THEN** initialization fails with the path and ignore-rule diagnostic guidance, does not force-add it, and does not leave a workspace presented as usable

### Requirement: Explicit adoption of existing documents
`init --adopt [--name <name>]` MUST initialize an existing unmanaged `devdocs/` directory in place, preserve every pre-existing file's bytes, pathname, permissions, and symlink target, and snapshot its non-ignored content in the initial docs commit without uploading. It MUST create only missing metadata/templates; any existing case-insensitive equivalent of README.md or FYI.md MUST suppress that template, even on case-sensitive filesystems. Existing ignored files remain untouched and omitted from the snapshot, with visible diagnostics. Ordinary init MUST refuse an unmanaged directory with guidance to use `--adopt`.

Adoption MUST retain the existing code-repository isolation checks and refuse a symlinked docs root, tracked docs paths, reserved Devdocs metadata, or existing/nested Git metadata rather than overwrite or inherit another repository. It MUST NOT copy/move the existing document tree, force-add ignored content, or alter the source index/HEAD. A failure MUST remove only initialization-created entries, leaving pre-existing data available for a corrected retry. Repeated adoption of a valid workspace MUST retain identity/history; `--adopt` on an absent docs path has ordinary fresh-init behavior.

#### Scenario: Migrate existing notes offline
- **WHEN** an untracked unmanaged docs directory contains notes, scripts, and nested directories and the user explicitly adopts it
- **THEN** their contents, names, and permissions remain unchanged, non-ignored content is committed locally, and no network request or source-repository commit occurs

#### Scenario: Preserve lowercase or custom templates
- **WHEN** existing documents include fyi.md and readme.md, with custom contents or applicable ignore rules
- **THEN** adoption does not create FYI.md/README.md aliases or overwrite those documents, and normal ignored-file rules remain effective

#### Scenario: Unsafe directory or failed initialization
- **WHEN** adoption encounters tracked paths, a symlinked root, reserved metadata, nested Git metadata, ignored required new metadata, or a failed initial commit
- **THEN** it fails without changing pre-existing data or the source index and does not leave a usable-looking partial workspace after ordinary command failure

#### Scenario: Repeat adoption or ordinary init refusal
- **WHEN** adoption is repeated on a valid workspace, or ordinary init encounters unmanaged documents
- **THEN** repeat adoption leaves identity/history unchanged, and ordinary init requires explicit adoption rather than silently taking over files

### Requirement: Explicit attachment and discovery
`list` MUST list attachable `devdocs/` branch refs without cloning content. `attach <project-branch>` MUST clone only that branch with full project history and no tags, validate versioned identity against the selected branch, and report declared links without applying them. Failure MUST NOT overwrite pre-existing content or leave a usable-looking partial workspace. Reattachment of the same valid workspace MUST be idempotent; a different or unmanaged path MUST be refused.

#### Scenario: Select one remote project (A3)
- **WHEN** a remote contains projects A and B and the user attaches A
- **THEN** only A's branch/history is requested and checked out and no root link is automatically created

#### Scenario: Missing branch or unavailable remote
- **WHEN** discovery or attachment fails because the remote is unavailable or the selected branch is absent
- **THEN** the command fails instead of returning an empty success or creating a new project

### Requirement: Checkout-independent identity and platform support
The CLI MUST support macOS and Linux, linked code worktrees, paths containing spaces/non-ASCII, invocation from inside its docs checkout, checkout relocation, and code branch switching without changing docs identity. Different code checkouts MUST hold independent docs replicas, even when attached to the same project.

#### Scenario: Worktree, moved checkout, and docs descendant (A14)
- **WHEN** a workspace is used in a linked worktree, moved with its code checkout, invoked from a docs descendant, or used after changing code branches
- **THEN** the CLI identifies the containing code root and same docs project without relying on an absolute synchronized machine path
