# Verification evidence

## Implementation and model

Implemented the complete eight-command CLI in Go with the standard library. The user approved `google-antigravity/gemini-3.8-flash:high` before implementation. The task launcher rejected its advertised per-call model argument; the approved worker was launched using a session-only `task.agentModelOverrides.task` setting. No model substitution or persistent model-configuration change was made. The parent authored OpenSpec/user documentation, reviewed the handoff, repaired integration defects, and ran the final checks.

## Executed checks

| Check | Observed result |
|---|---|
| `go test -race ./...` on macOS arm64, Go 1.27.3, Git 2.56.0 | PASS |
| `go vet ./...` | PASS |
| Native `go build -o bin/devdocs ./cmd/devdocs` | PASS |
| Linux arm64 cross-build | PASS |
| `go test -race ./...` inside `golang:1.24-bookworm`, `GOTOOLCHAIN=local`, network disabled | PASS |
| Compiled CLI A1–A17 scenario harness on macOS arm64 | PASS |
| Same compiled-CLI scenario harness on Linux arm64 with Git 2.39.5, network disabled | PASS |
| Additional boundary harness on macOS | Eight groups PASS, including actual OpenSpec integration |
| Additional boundary harness on Linux | Seven groups PASS; OpenSpec executable unavailable in that image |
| `openspec validate implement-devdocs-cli --strict` | PASS |

All behavioral fixtures used disposable code repositories, bare remotes, isolated HOME/XDG/Git configuration, and synthetic content. Real user projects, credentials, global Git configuration, and hosting remotes were not used. Throwaway smoke harnesses are removed after recording evidence; permanent Go regression tests remain.

## CLI acceptance evidence

- **A1/A2:** Init with the configured remote temporarily unavailable, from a code descendant; repeat init from inside docs; same identity and docs HEAD; unpublished status; unchanged source HEAD/index and clean source status.
- **A3:** A second independent project existed in the same bare remote. Attaching the first did not fetch the second project's ref or commit. Repeat attachment remained idempotent.
- **A4/A5:** Additions, exact edited contents, and deletions reached a second replica. Repeating clean sync did not create a commit.
- **A6:** Different-file edits on two replicas merged and retained both changes.
- **A7:** Same-line edits produced a real Git conflict, named paths, and resolution/abort guidance. A pending merge blocked another sync; abort retained the snapshot; manual resolution committed and published successfully.
- **A8:** Making the remote unavailable after an edit produced a nonzero sync result while retaining the new file in the local commit and working tree. Offline status worked; restoring the remote allowed explicit publication.
- **A9/A15:** A disposable Git transport wrapper paused an actual push. A second mutation in that checkout was rejected by the lock. Another replica advanced the remote; releasing the first push caused a normal non-force rejection. Explicit retry retained both replicas' files.
- **A10:** File and nested-directory mappings created relative symlinks. Attach did not install links; explicit apply did. Source content remained in docs and source repository status stayed clean.
- **A11:** Tracked/untracked collisions, raw traversal, reserved metadata, destination overlap, and symlink-parent escape attempts were refused without replacing existing content.
- **A12:** A project ignore negation blocked initialization. Force-adding an installed instruction link to the source index blocked status and sync; Devdocs did not silently untrack it.
- **A13:** A global FYI.md ignore caused explicit initialization failure and owned-staging cleanup. An ignored user note remained visible in status and was not force-added.
- **A14:** Attach and status worked in a linked worktree with a `.git` file. Moving a normal checkout and switching code branches preserved project identity and relative links.
- **A16:** A manually replaced link destination survived unlink refusal. Removing its declaration on one replica did not delete the other replica's link; status reported stale ownership and explicit unlink preserved the source.
- **A17:** A tampered local identity was refused. A deleted previously published remote branch was not silently recreated.

## Additional boundaries and repairs

The first implementation failed the installed-link/index isolation check and allowed a source symlink to expose an external file. Both failures were reproduced using the actual binary, fixed, and subsequently passed the complete runtime harnesses. Permanent regression tests cover these failures.

Additional passing checks covered direct/nested source symlinks into external or reserved metadata, safety checks on idempotent link application, actual branch/origin/pushurl pins before staging, observed-upstream publication state, symlinked metadata refusal, status/sync of unapplied declarations, and multi-link preflight collisions.

A focused binary check reproduced `status` truncating `FYI.md` to `YI.md` and classifying an unstaged edit as staged. NUL-delimited Git status parsing repaired it; the binary then reported the full pathname as unstaged. Regression coverage also checks a staged rename containing spaces and Unicode.

An actual quoted editor executable with arguments received the configuration pathname through `config --edit`; a literal `$(touch ...)` argument was not evaluated, and VISUAL took precedence over EDITOR. Changing the global default remote did not retarget an existing workspace. Disabling the Git receive service made a clean sync fail nonzero without changing local HEAD rather than incorrectly claiming publication success.

OpenSpec 1.9.0 was initialized separately inside a disposable docs checkout. After `devdocs link openspec openspec`, `openspec list --specs --json` from the code root discovered the fixture capability through that symlink.

## Limits of the evidence

- Runtime coverage is arm64 macOS and arm64 Linux. Other architectures are not claimed as runtime-tested.
- SSH/HTTPS authentication and live GitHub/GitLab hosting were not exercised; actual transfer tests used local Git file transport.
- Linux OpenSpec integration was skipped because OpenSpec was not installed in the test image. macOS OpenSpec integration passed; other agents/editors and instruction precedence are not implied.
- The acceptance matrix represents the exercised scenarios above, not exhaustive failure-injection at every filesystem instruction or compatibility with every Git configuration.
- No private-remote visibility, encryption, or data-loss-prevention guarantee is claimed. The change remains active for review and is not automatically archived.

## Existing-data adoption

Added `init --adopt` after the user encountered an existing unmanaged docs directory. This feature was implemented inline; the original model description above applies to the initial delegated implementation.

The full `go test -race ./...` suite passes on macOS and Linux with the Go 1.24 baseline, and `go vet ./...` passes. The rebuilt installed command resolves through the existing symlink to `bin/devdocs`; no reinstall or user-configuration change was needed.

A new disposable actual-binary harness passed all four groups on macOS arm64 and Linux arm64/Git 2.39.5:

1. Offline adoption from a descendant directory, with the remote unavailable. Existing file contents, inodes, permissions, mtimes, and symlink targets were unchanged. Lowercase fyi.md/readme.md were retained without uppercase aliases, repeat adoption preserved history, and source HEAD/index were unchanged.
2. Explicit sync followed by attachment on a second checkout reproduced the adopted non-ignored bytes, executable permissions, and symlink targets. An ignored synthetic environment file remained local and was reported as omitted.
3. A deliberately failing Git pre-commit hook caused initial commit failure. Only generated entries were removed; the original notes retained their contents and inode. A retry succeeded after correcting the configured hook.
4. Tracked docs, nested Git metadata, reserved metadata, a symlinked root, and ignored required new metadata were refused without changing original notes or the source index.

Permanent adoption regressions also cover ignored existing templates and initial required-file diagnostics. No adoption, sync, or content changes were performed in any real user project. No real environment-file contents were read. Existing privacy/transport/platform limits above still apply.

An additional actual-binary check confirmed that `init --adopt` on an absent docs path creates a fresh workspace, ordinary repeat init preserves its HEAD, and initialization does not persist an absolute `core.worktree` path.

## Native Git pass-through

Added `devdocs git <args...>` inline as the ninth CLI command. The full race-enabled regression suite passes on macOS and Linux/Go 1.24, and vet/native/Linux builds pass. The installed command uses the rebuilt executable through its existing symlink.

A disposable actual-binary harness passed six groups on both macOS arm64 and Linux arm64/Git 2.39.5:

1. Binary stdin to Git hashing, native stdout/stderr, global flags, a real configured Git alias, and exact nonzero exit codes 42 and 128. Quiet diff also retained exit code 1 with no added wrapper output.
2. Literal Unicode/leading-dash path arguments, forced staging of an ignored private file, and a commit message containing a literal shell expression without evaluating it. Source history/index stayed unchanged and nothing was uploaded.
3. Native interactive `git add --patch` consumed a supplied response and staged the selected hunk.
4. A real merge conflict could be aborted through `devdocs git merge --abort` from a docs descendant.
5. Native Git restored malformed metadata and recovered the versioned metadata file even when both Devdocs marker files were absent.
6. Absent, plain, or symlinked docs/Git directories were refused without staging untracked source files.

Permanent regressions cover private-only commits, source-index fallback prevention, and invalid/absent metadata recovery. Managed recovery errors and documentation now use `devdocs git`. This raw manual escape hatch deliberately retains Git capabilities, output, aliases/hooks, and caller-selected repository flags rather than applying managed synchronization safeguards.

A focused read-only smoke reproduced Git falling back to the source repository when docs/.git existed but was empty/invalid. The wrapper now asks Git for its actual absolute Git directory and rejects any mismatch before running user arguments. The same fixture subsequently failed safely with no native stdout, and permanent source-index protection coverage includes invalid Git directories. A read-only check through the installed command in a real user checkout reported its expected devdocs root; no user documents or index were changed.

## First public release

A first-user walkthrough with the compiled binary reproduced three defects, each covered by a regression test confirmed to fail before its fix: a nested repository synced as an empty gitlink while reporting success, a `.devdocs.json` conflict surfaced as a JSON parse error in `status` and `sync`, and subcommand `--help` exited 1 with an error.

After the fixes, `go test -race ./...`, `go vet ./...`, and gofmt pass on macOS arm64, and the race-enabled suite passes on Linux arm64 with Go 1.24.13 and Git 2.39.5. Following the printed recovery steps for a metadata conflict published both link declarations, and both replicas applied them with a clean code repository. `git clean -fdx` left `devdocs/` in place, `git clean -ffdx` removed it, and `attach` restored the synced content.

The release build commands, run in a Linux container with Go 1.26, produced all four binaries and checksums. The darwin/arm64 binary is ad-hoc signed, contains no local paths, and ran here reporting 0.1.0. A substituted curl served those files to `install.sh`, which installed on macOS arm64 and Linux arm64 under dash, replaced a symlink without changing its target, and stopped with no installed file on a tampered binary, empty checksums, and a 404. A script truncated mid-function failed with a syntax error and did nothing. actionlint and shellcheck report nothing.

### Limits

- The GitHub Actions workflows have not run, and `install.sh` has not downloaded from real GitHub Releases, because the public repository does not exist yet. Task 7.5 covers both.
- The amd64 binaries have not been executed. The verify-install job is their first run.
- Live GitHub SSH transport has been used for `list` and earlier real project sync. HTTPS remotes remain unexercised.
