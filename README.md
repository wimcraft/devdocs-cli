# devdocs

_This is a story about Wim_ 🧑‍💻.

_Wim writes code, often with an AI agent_ 🤖.

_Wim keeps plans, decision logs, drafts and one brutally honest `TODO.md` right next to the code_ 📝.

_None of that belongs in the team's repository_ 🙅.

_This is what happens to Wim every week_ 🚧:

* _Wim writes a brilliant plan in `notes.md`._ 😀
* _Wim can't commit it, because the team doesn't need to read "rewrite this mess"._ 🤔
* _Wim hides it from Git and moves to the other laptop. The plan stayed behind._ 🙁
* _Wim runs `git clean -fdx` to fix a weird build. The plan is now gone forever._ 😢

_Don't be like Wim, use devdocs!_ 🎉

`devdocs` gives your code checkout a private `devdocs/` folder. It looks like any other folder, but it is a real Git repository with its own history, and it syncs to a private remote of your choosing. Your code repository never notices: no `.gitignore` edits, no commits, nothing for reviewers to raise an eyebrow at.

```console
$ cd my-app
$ devdocs init
$ echo "Rewrite everything. Tell nobody." > devdocs/plan.md
$ devdocs sync -m "Plan the rewrite"

# Later, on the other laptop
$ cd my-app
$ devdocs list
$ devdocs attach devdocs/my-app-6c116702-5f3a-4e9e-a6d6-e6ada633a166
$ cat devdocs/plan.md
Rewrite everything. Tell nobody.
```

That's it!

Oh, one more thing: a single private remote holds all your projects, one branch each, and a machine only downloads the projects you ask for.

Hold on, you can also [link](#link-files-into-the-code-repository) files like `AGENTS.md` or `.claude/skills` from `devdocs/` into your code checkout, and Git won't see those either. Your agent gets its instructions, your teammates get a clean diff.

And no magic: there is no daemon and no auto-sync. Nothing leaves your machine until you run `devdocs sync`, and it's plain Git underneath, so there is nothing to be locked into.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/wimcraft/devdocs-cli/main/install.sh | sh
```

This installs a checksum-verified binary for macOS or Linux (amd64 or arm64) to `~/.local/bin`. Run it again to upgrade.

<details>
<summary>Pin a version, pick a directory, build from source, or uninstall</summary>

```bash
# Pin a release and choose the directory
curl -fsSL https://raw.githubusercontent.com/wimcraft/devdocs-cli/main/install.sh | DEVDOCS_VERSION=v0.1.0 DEVDOCS_INSTALL_DIR=/opt/bin sh

# Build from source with Go 1.24+
go install github.com/wimcraft/devdocs-cli/cmd/devdocs@latest
```

To uninstall, delete the binary and `~/.config/devdocs`. Your docs stay in their checkouts and in your remote.

</details>

You need Git 2.39+, a Git identity (`user.name` and `user.email`), and a private Git remote you can already push to. Nothing else: no Go, Node or Python at runtime.

## Getting started

Create one private repository, once. It will hold all your projects.

```bash
gh repo create my-devdocs --private    # or use any private Git remote
devdocs config --remote git@github.com:you/my-devdocs.git
```

Then, in any code repository:

```bash
devdocs init                 # works offline, uploads nothing
# Edit devdocs/FYI.md, or add any files you like
devdocs status               # see what changed
devdocs sync -m "First notes"
```

On another machine or checkout, set the same remote, then run `devdocs list` and `devdocs attach <branch>`. Use `attach` here, not `init`: `init` always starts a brand new project.

Devdocs doesn't create the remote or set up SSH keys or tokens; it uses your normal Git authentication. Please don't put tokens or passwords in remote URLs. Devdocs refuses most of them anyway.

## Commands

| Command | What it does |
|---|---|
| `devdocs config [--remote <url> \| --edit]` | Show or set the default remote for future projects. |
| `devdocs init [--adopt] [--name <name>]` | Start a new project here, offline. |
| `devdocs list` | Show the projects on your remote without downloading them. |
| `devdocs attach <branch>` | Download one existing project into this checkout. |
| `devdocs status` | Show changes, ignored files, sync state and link health, offline. |
| `devdocs sync [-m <message>]` | Commit everything in `devdocs/`, merge the remote, and push. |
| `devdocs git <args...>` | Run plain Git inside `devdocs/`, from anywhere in the checkout. |
| `devdocs link <source> <destination>` | Symlink a file or folder from `devdocs/` into the code checkout. |
| `devdocs link --apply` | Install the links declared by this project on this machine. |
| `devdocs unlink <destination>` | Remove a link and its declaration, keeping the source file. |

Flags go after the subcommand and before positional arguments. Configuration lives at `$XDG_CONFIG_HOME/devdocs/config.json`, or `~/.config/devdocs/config.json`. `--remote` only affects future projects, never existing ones.

## Recipes

### Already have a `devdocs/` folder?

Adopt it. Nothing is copied, moved, renamed or overwritten, and nothing is uploaded until you sync.

```bash
# Check .gitignore and look for anything sensitive first
devdocs init --adopt
devdocs status
devdocs sync    # when you're ready
```

<details>
<summary>The fine print</summary>

Adoption creates a new, independent docs history in place. Non-ignored files go into the local first commit; ignored files stay untouched and are reported as omitted. Only missing metadata and templates are created, so an existing `fyi.md` or `readme.md` keeps its name and contents. Scripts keep their permissions, and symlinks are recorded as links rather than followed.

Tracked docs paths, a symlinked docs root, an existing `.devdocs.json`, and existing or nested `.git` entries are refused. Adopting a valid workspace again changes nothing. If initialization fails, only generated entries are rolled back. Don't edit the directory while adoption runs, and after a hard process kill, inspect any generated metadata before retrying.

This migrates an unmanaged folder. To connect to an existing Devdocs project, use `attach`.

</details>

### Link files into the code repository

Agents and tools expect their files in specific places. Keep the real file in `devdocs/` and link it:

```bash
# Create devdocs/AGENTS.md and devdocs/skills/ first
devdocs link AGENTS.md AGENTS.md
devdocs link skills .claude/skills
devdocs sync
```

Sources are relative to `devdocs/`, destinations to the code checkout root. Links are recorded in the project, but other machines only get them when you ask, after you've looked at them:

```bash
devdocs link --apply
```

<details>
<summary>The fine print</summary>

Files and whole directories can be linked. Existing files, directories and foreign links are never replaced or merged, and tracked files stay the code project's business: Devdocs refuses to overwrite them.

`attach` and `sync` never install links. Devdocs doesn't install OpenSpec, agent skills, commands or hooks, and doesn't run synchronized scripts. Individual agents and sandboxes may restrict linked content, so no particular agent version is promised to work.

`unlink` keeps the source and the local exclusion. A manually replaced destination is left alone. Removing a declaration on one machine never deletes a link on another; `status` reports it as stale so you can decide.

</details>

### When sync can't merge

Two machines edited the same line. It happens. Devdocs never picks a winner, rebases or force-pushes; your snapshot is safe in local history, and `sync` prints the exact paths:

```bash
# Fix the conflicting files, then:
devdocs git add -- path/to/resolved-file
devdocs git commit
devdocs sync

# Or give up on the merge. Your snapshot stays in history:
devdocs git merge --abort
```

Adding links on two machines between syncs conflicts in `.devdocs.json`. Keep both entries in its `links` array and follow the same steps.

Network or push failure? Your commit stays local, so fix the cause and run `devdocs sync` again.

<details>
<summary>The fine print</summary>

`sync` takes everything: staged and unstaged changes, new non-ignored files, and deletions. It isn't a partial commit, so finish editing first. The lock stops other Devdocs commands, not your editor. A clean sync creates no commit.

A Git repository nested inside `devdocs/` is refused, because Git would record it without its files. Remove its `.git` directory or ignore it.

A pending merge, rebase or cherry-pick must be finished or aborted first. If a previously published remote branch disappears, Devdocs refuses to quietly recreate it.

Don't retarget the docs `origin`, switch its branch, or edit its UUID or branch metadata to move projects. Machine-local pinned state rejects those changes on purpose.

</details>

### Plain Git, when you need it

`devdocs git` runs Git in the docs repository, from anywhere in the code checkout:

```bash
devdocs git log --oneline
devdocs git add -f -- fyi.md
```

<details>
<summary>The fine print</summary>

All arguments go straight to Git, and streams and exit codes are preserved. It still works with broken metadata or an unfinished merge, and refuses missing or symlinked docs or Git directories rather than falling back to your code repository.

This is direct Git, not synchronization: no snapshot, upload, link application or lock. Don't run mutations alongside `sync`. Aliases, hooks and destructive commands behave as they normally do.

</details>

### Missing files? Check the ignore rules

Ignored files don't sync, and that includes rules from your global Git configuration. `devdocs status` lists them.

```bash
devdocs git check-ignore -v -- FYI.md
git config --show-origin --get core.excludesFile
```

<details>
<summary>The fine print</summary>

If a required new template such as `FYI.md` is ignored, `init` fails rather than force-adding it; adoption keeps ignored existing templates without adding them. Devdocs doesn't change global ignore rules.

In the code repository, tracked paths are never protected by ignore rules, and a project `.gitignore` can override local exclusions. Devdocs refuses unsafe isolation rather than untracking files or changing shared rules. Exclusions live in `.git/info/exclude` and are shared by linked worktrees, while each checkout or worktree has its own independent docs replica.

</details>

## Good to know

Switching code branches doesn't switch docs projects. Renaming or moving the checkout doesn't change a project's identity. Fork names and code remote URLs never select a project automatically.

The jokes stop here, because this part matters:

- Use a **private, authorized** remote. Devdocs can't check that it's private, and owning the repository doesn't make it OK to upload your employer's or client's material.
- Don't store secrets, private keys, credentials or production dumps. There's no encryption, secret scanner or data-loss prevention.
- Every project on a remote shares its access boundary. Selective cloning is not per-project access control.
- Deleted files stay in Git history. Deleting a leaked credential doesn't fix the leak; rotate it.
- Exclusions prevent common accidents, not `git add -f`, plumbing commands or other processes. Hooks aren't a security boundary.
- Unsynced files have no backup. `git clean -ffdx` removes `devdocs/` entirely, even though `git clean -fdx` skips it.

Windows, background sync, agent installers, multiple remotes per project and remote deletion aren't included.

## Development

```bash
go build -o bin/devdocs ./cmd/devdocs
go test -race ./...
go vet ./...
openspec validate implement-devdocs-cli --strict
```

Pushing a `v*` tag tests, builds and publishes the release, then verifies `install.sh` against it on every supported platform. The proposal, design, specifications and verification evidence are in [`openspec/changes/implement-devdocs-cli/`](openspec/changes/implement-devdocs-cli/). OpenSpec's generated skills and commands are in `.omp/`.

## License

[MIT](LICENSE)
