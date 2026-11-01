## 1. Runtime and workspace lifecycle

- [x] 1.1 Create the Go module, CLI entrypoint, argument parsing, help/version, safe Git runner, and validated global configuration/editor handling.
- [x] 1.2 Implement code-root discovery, Git version checks, atomic metadata/local-state storage, UUID branch identity, and process locking.
- [x] 1.3 Implement effective escaped local exclusions and tracked-path checks without modifying the code index or tracked files.
- [x] 1.4 Implement offline independent-history init, required ignored-template diagnostics, idempotence, and owned-staging cleanup.
- [x] 1.5 Implement remote ref listing and selected-branch attach with pinned identity/remote validation and no implicit links.

## 2. Git synchronization

- [x] 2.1 Implement snapshots of all non-ignored changes, scoped fetch, remote identity/ancestry checks, fast-forward/normal merge, and explicit non-force push.
- [x] 2.2 Implement publication-state integrity, missing-remote-branch refusal, pending-operation checks, conflict recovery guidance, and retained local commits on transport failure.
- [x] 2.3 Validate metadata/link declarations before and after integration and report concurrent unpublished local edits.

## 3. Safe paths and visibility

- [x] 3.1 Implement link declaration/source/destination validation, preflight/rollback, relative symlinks, idempotence, and local ownership.
- [x] 3.2 Implement explicit apply and non-destructive unlink including stale ownership after remote declaration removal.
- [x] 3.3 Implement offline status with ignored/changed files, stale ahead/behind state, Git operations, safety failures, and missing/changed/stale link health.

## 4. Regression coverage and runtime verification

- [x] 4.1 Add deterministic isolated regression coverage for plausible data-loss, identity, ignore precedence, path escape/collision, conflict, and publication bugs; skip build/test/format runs until integration.
- [x] 4.2 Run the compiled CLI through two-replica bare-remote scenarios covering A1–A17; retain exact outcomes in verification evidence and fix observed failures.
- [x] 4.3 Run formatting, Go tests/race checks, vet, current-host build, Linux cross-build, and strict OpenSpec validation after implementation integration.
- [x] 4.4 Document build/install, command usage, first-run workflow, ignored-file diagnostics, manual recovery, supported platforms, and privacy/safety limits.

## 5. Existing-data adoption

- [x] 5.1 Add explicit init --adopt parsing, in-place initial snapshot, case-insensitive template preservation, and owned-entry rollback.
- [x] 5.2 Cover original-data preservation, ignore rules, unsafe directories, failed commit recovery, and idempotence with regression and actual-binary scenarios.
- [x] 5.3 Update usage, migration instructions, release notes, and OpenSpec verification evidence; run macOS/Linux checks.

## 6. Native Git access

- [x] 6.1 Implement devdocs git with direct streams, verbatim arguments, native exit codes, and safe docs-checkout selection.
- [x] 6.2 Verify actual Git commits, stdin/global flags/path arguments, exit codes, metadata/conflict recovery, and missing-checkout isolation.
- [x] 6.3 Migrate recovery examples/help to devdocs git and update specifications, usage, and release notes.

## 7. First public release

- [x] 7.1 Refuse nested repositories in sync, report unfinished Git operations before metadata errors, and make subcommand help exit zero, with regression coverage.
- [x] 7.2 Set the public module path, tag-derived version, license, and CI and release workflows.
- [x] 7.3 Add install.sh with checksum verification and exercise success, tampering, missing checksums, download failure, truncation, and symlink replacement.
- [x] 7.4 Rewrite the README around the problem it solves and update specifications and release notes.
- [ ] 7.5 Publish the repository, tag v0.1.0, and confirm the verify-install job passes on all four platforms.
