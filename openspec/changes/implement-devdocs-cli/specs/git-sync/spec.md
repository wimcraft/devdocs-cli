## Purpose

Publish and integrate one project's private context through ordinary Git history while retaining recoverable local work across conflicts and transport failures.

## ADDED Requirements

### Requirement: Explicit workspace-wide snapshot and scoped transfer
`sync [-m <message>]` MUST snapshot all non-ignored additions, staged/unstaged changes, and deletions in the docs repository, fetch only the expected project branch without tags, fast-forward or normally merge remote history, and push only that branch without force. It MUST NOT rewrite commits or touch another project's branch. No-change sync MUST NOT create a commit. Success MUST require an accepted remote update or verified up-to-date push. Remaining concurrent local edits MUST be reported as unpublished. Before staging, sync MUST refuse an untracked non-ignored nested Git repository, because Git would record it as a gitlink and a reported success would upload none of its files.

#### Scenario: Transfer edits and deletions (A4, A5)
- **WHEN** the user syncs local additions, edits, and deletions and then attaches or syncs another machine
- **THEN** the second workspace receives exactly the committed changes and a subsequent clean sync creates no commit

#### Scenario: Nested repository in the docs
- **WHEN** the docs contain an untracked non-ignored nested Git repository
- **THEN** sync fails naming that path, stages and uploads nothing from it, and succeeds once it is ignored or no longer a repository

#### Scenario: Nonconflicting divergence (A6)
- **WHEN** two replicas commit changes to different files
- **THEN** sync integrates both histories without losing either change

### Requirement: Recoverable conflicts and transport failures
The CLI MUST preserve local snapshot commits and user files on fetch/push failure. Conflicts MUST remain visible as ordinary Git merge state, name affected paths, and provide manual resolution and merge-abort commands; no push occurs. Existing merge/rebase/cherry-pick/revert state or unmerged entries MUST block a new mutation requiring a clean Git operation state. Sync MUST check for such state before reading metadata, so conflict markers in `.devdocs.json` report the unfinished operation rather than a parse error. No automatic reset, stash disposal, conflict-side selection, or retry loop is permitted.

#### Scenario: Conflicting edit or deletion (A7)
- **WHEN** remote integration conflicts with a local edit or deletion
- **THEN** sync returns nonzero with conflicting paths and exact Git recovery guidance and retains the local snapshot

#### Scenario: Conflict in metadata
- **WHEN** link declarations added on two replicas conflict in `.devdocs.json`
- **THEN** later `sync` and `status` commands report the unfinished merge, and resolving it through `devdocs git` lets sync publish both declarations

#### Scenario: Offline or rejected upload (A8, A9)
- **WHEN** a fetch fails or a remote update causes push rejection after the snapshot
- **THEN** local history remains intact, the command returns nonzero, and the user can explicitly retry sync

### Requirement: Identity and publication integrity
Before snapshot and after integration the CLI MUST validate schema, UUID, branch, pinned remote, and all link declarations. It MUST reject unrelated history or a remote project identity mismatch before merging. A never-published branch can be created remotely; a previously observed published branch disappearing MUST be an error rather than permission to recreate it.

#### Scenario: Missing or wrong project (A17)
- **WHEN** an established remote branch vanishes, presents a different identity, or has unrelated history
- **THEN** sync refuses without publishing or merging the wrong project

#### Scenario: Invalid remote declarations
- **WHEN** remote metadata contains an unsafe link declaration or an integration produces invalid metadata
- **THEN** sync fails without applying links or pushing invalid metadata and retains diagnosable Git state

### Requirement: Exclusive local mutation and recoverable interruption
Managed mutating commands (`init`, `attach`, `sync`, `link`, and `unlink`) MUST use an exclusive per-checkout process lock that is released on process exit. They MUST refuse a concurrent owner clearly. Interrupted Git operations and partially completed initialization MUST be diagnosed or cleaned only when the command owns the temporary data, without deleting pre-existing user data. Explicit native Git commands are a manual recovery escape hatch rather than managed synchronization operations.

#### Scenario: Concurrent commands and interruption (A15)
- **WHEN** another command owns the workspace lock or an earlier Git operation was interrupted
- **THEN** the next managed command fails with a recoverable diagnosis instead of racing or discarding work

### Requirement: Native Git access in the docs checkout
`devdocs git <args...>` MUST run installed Git in the current project's real docs checkout, forwarding all arguments without Devdocs flag parsing or shell interpolation and connecting stdin, stdout, and stderr directly. It MUST return Git's exit code without replacing native errors with a generic Devdocs exit code. It MUST work from the code root, descendants, and docs descendants, including recovery when Devdocs metadata is invalid or absent. It MUST refuse missing, non-repository, or symlinked docs/Git directories rather than let Git fall back to the code repository.

This explicit escape hatch MUST retain native Git behavior, including user-selected hooks, aliases, and output, without requiring valid metadata, a clean merge state, or the Devdocs synchronization lock. It MUST NOT perform automatic sync, snapshot, upload, or link application. Users are responsible for native Git operations and explicit Git flags that select another repository.

#### Scenario: Inspect or commit private documents
- **WHEN** the user runs native Git status, add, diff, or commit through `devdocs git` from a supported working directory
- **THEN** the default operation uses the docs repository rather than the source repository, without implicitly synchronizing it

#### Scenario: Recover malformed metadata or conflicts
- **WHEN** Devdocs metadata is invalid/missing or a merge needs explicit resolution or abort
- **THEN** native Git recovery remains available through the command without Devdocs metadata/operation validation

#### Scenario: Native input and result handling
- **WHEN** Git reads stdin, accepts global flags or literal path arguments, writes an error, or exits nonzero
- **THEN** native input/output and exit semantics are preserved

#### Scenario: No valid docs checkout
- **WHEN** no real nested docs Git checkout exists or its root/Git directory is symlinked
- **THEN** the command fails without modifying the source repository's index or history
