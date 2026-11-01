## Context

This is an empty implementation repository with initialized OpenSpec. See proposal.md for motivation and the six capability deltas for contracts. The approved worker is `google-antigravity/gemini-3.8-flash:high`; the parent owns specification, integration, and runtime verification.

## Goals / Non-Goals

**Goals:** a boring, recoverable Git wrapper with no copy mirror, no global Git mutations, and strict path/identity checks. Build with Go 1.24+ standard library; runtime Git 2.39+ on macOS/Linux.

**Non-Goals:** provider APIs, secrets management, agents/hooks/agent installers, automatic publication/linking, Windows, and multiple stores. OpenSpec here describes development of Devdocs; the Devdocs binary itself does not install or drive OpenSpec.

## Decisions

### Executable and modules

Use module `github.com/wimcraft/devdocs-cli`, entrypoint `cmd/devdocs`, and a small `internal/devdocs` package split by CLI/config, Git/workspace, sync, exclusions/paths, and links. Use standard `flag`, `os/exec`, JSON, crypto/rand, and Unix file locking; avoid a dependency framework. Managed commands return errors to a single entrypoint and exit nonzero on failure. Provide `--help`/`--version`; help exits zero, including on subcommands, and the version is injected from the release tag, defaulting to `dev`. Managed command flags follow the subcommand and precede positional arguments. Editor settings support quoted executable/arguments through a non-evaluating tokenizer; no `sh -c`.

`devdocs git` bypasses Devdocs flag parsing and managed-operation validation: reuse root discovery and the sanitized Git command runner, require real docs/.git directories to prevent parent-repository fallback, and wire streams directly rather than buffer/transform native output. Preserve child exit codes and Unix signal-derived status. Recognize the conventional immediate nested docs checkout during recovery even if both Devdocs marker files are gone. Do not acquire a managed sync lock or validate metadata/merge state: those checks would prevent recovery. Native Git flags/aliases/hooks remain explicit user-controlled behavior, not automatic Devdocs execution.

### Metadata and pinned local identity

Schema 1 `.devdocs.json`: `schema_version`, `project_id` (UUID v4), `branch`, `name`, `created_at` (RFC3339 UTC), and `links` containing `{source,destination}`. Use atomic temp-write/rename and reject unsupported schemas, invalid UUIDs/branch mismatch, duplicate/overlapping paths, and unsafe mappings. Repositories remain inspectable with ordinary Git. Optional directories are user-created.

Inside the docs `.git`, keep an atomic `devdocs-state.json` containing pinned UUID/branch/remote, `published` boolean, and installed-link ownership records. Attach marks publication observed; init starts false; observing the upstream marks true even when a subsequent push fails. Pinning rejects remote retargets or metadata identity edits. State has no absolute project path so moves work. Never synchronize it.

### Git execution and history

Execute Git using argument arrays and bounded useful diagnostics; redact remote userinfo and do not expose credential-bearing command lines. Reject credential-bearing HTTPS and option-like remotes; support SSH, HTTPS, `file://`, and local filesystem paths for ordinary Git/testing. Pass `--` where accepted, validate branch refs, and remove inherited repository redirection environment (`GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`, object directories) so nested operations cannot change the wrong repository. Preserve deliberate Git identity/config/credential settings.

Create init in an owned staging directory under the code repository's actual Git metadata, then rename into the excluded destination; do not delete user-owned paths. Configure exactly one fetch refspec and `remote.origin.tagOpt=--no-tags`. Clone attach with `--single-branch --no-tags --branch` into owned staging, validate, then rename. Local transport experiments use file URLs to avoid clone hardlink optimizations importing unrelated objects. Required initial files are checked with Git ignore diagnostics before staging. Require an existing author identity (code-local identity may be copied to docs-local config, never global).

For `init --adopt`, preserve the existing worktree in place and prepare only Git metadata in the owned staging directory. Reject indexed docs, reserved metadata, symlinked roots, and nested `.git` entries before mutation. Detect existing README.md/FYI.md aliases with case-insensitive root-entry comparison, not filesystem-dependent existence checks. Create only missing files with exclusive creation; keep rollback ownership records for those new files. Use Git's explicit `--git-dir`/`--work-tree` arguments to snapshot existing non-ignored content without copying or following file symlinks. Only after a successful initial commit and local state write, move the prepared `.git` into the existing directory; do not persist an absolute core.worktree path. On ordinary failure remove only entries created by this attempt, leaving originals intact. A hard process kill can leave newly generated files requiring inspection; never delete pre-existing files to recover automatically.

Before sync, enforce identity, refspec/remote, index isolation, and clean operation state. Snapshot with `git add -A` only in docs. Use `ls-remote --heads` to distinguish absent branch from transport failure, fetch the exact branch, validate remote metadata from its commit, verify a merge base, merge normally, revalidate, and push explicit `HEAD:refs/heads/<branch>`. New branches do not use force or force-with-lease. Existing upstream disappearance is not silently recreated. Conflicts remain for explicit Git resolution/abort. Report changes left by concurrent editors after push; no retry engine.

### Root discovery, exclusions, and locks

Resolve the current Git top-level; if it is a recognized docs checkout, use its parent as the containing code root and verify that parent with Git. Never climb through arbitrary nested repositories. Obtain the common Git directory and `info/exclude` using Git plumbing, including worktrees. Local excludes use escaped literal root-anchored paths in recognizable managed entries, preserving unrelated content. Detect indexed paths with NUL-delimited literal pathspecs and verify effective exclusion with `git check-ignore --no-index`. Refuse symlinked managed metadata/unsafe exclusion files rather than following them.

Use nonblocking Unix `flock` on a per-root lock file under the code common Git directory (hash the canonical code root), so init/attach can lock before the docs `.git` exists. The descriptor releases on exit and lock files need no stale-PID deletion. Shared common exclude updates require a short separate common-directory lock to prevent lost updates across worktrees. Status is read-only. No hook configuration changes are needed in the source repository.

### Links and untrusted declarations

Normalize relative slash paths and reject traversal components, absolute paths, `.git` at any depth, reserved docs metadata, source escape, parent symlinks, and destination ancestor overlaps. Existing symlink content is never followed outside roots. Validate mapping syntax even before sources are present, and require live sources at application. Validate a directory source's symlink entries so a linked directory cannot expose external targets through a nested symlink. Source content is never copied or executed.

Preflight every apply destination and source before changes. Use relative symlink targets; create safe missing parent directories, tracking newly created entries for rollback on failure. Ownership is persisted locally only after successful application. A link created without local ownership is not silently adopted. Unlink compares the recorded target with `lstat/readlink`, supports stale owned links after remote declaration removal, and never removes changed paths or source content. Keep exclusions after unlink intentionally.

## Risks / Trade-offs

- One remote grants access to all project branches → document privacy boundary; branches are not ACLs.
- Git ignores can omit intended notes → show ignored files and fail explicitly for ignored templates, never force-add.
- Editors are not locked by Devdocs → snapshot semantics, retain concurrent changes, report unpublished remainder.
- Another process can race filesystem checks → validate parents close to mutation, use atomic metadata writes, never overwrite existing destinations.
- Shared worktree exclusions affect sibling worktrees → documented local common-repository scope and serialized updates.
- Arbitrary remote changes can contain bad metadata → pre-merge and post-merge validation; no automatic link execution.
- Cross-platform runtime coverage is limited to available hosts → run actual CLI on the current host and cross-build Linux; do not claim Linux runtime testing without a Linux runtime.

## Migration Plan

No existing application state to migrate. Users install a release with `install.sh`, or build from source with `go install github.com/wimcraft/devdocs-cli/cmd/devdocs@latest`. Releases use `go build` in a workflow loop and `gh release create` rather than a release tool: four raw binaries need no archive format, and unversioned asset names let `releases/latest/download` work without the GitHub API. Binaries are built with stable Go because the Go 1.24 minimum is out of support; CI tests both. Do not initialize a code Git repository, commit user changes, create hosting remotes, or install globally as part of implementation. Rollback is removal of the executable; docs remain ordinary Git repositories. Completed OpenSpec artifacts remain reviewable as an active change until the user chooses to archive.
