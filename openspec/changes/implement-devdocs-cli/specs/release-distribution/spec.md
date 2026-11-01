## Purpose

Let a macOS or Linux user install a verified `devdocs` binary from GitHub Releases with one command, without a Go toolchain.

## ADDED Requirements

### Requirement: Tagged release builds
Pushing a `v*` tag MUST run the test suite before publishing. The release MUST contain one static binary each for darwin and linux on amd64 and arm64, named `devdocs_<os>_<arch>`, plus a `checksums.txt` of their SHA-256 digests. Binaries MUST report the tag version without its `v` prefix through `devdocs --version` and MUST NOT embed local build paths. Builds from source MUST report version `dev`.

#### Scenario: Publish a tag
- **WHEN** a `v0.1.0` tag is pushed and the tests pass
- **THEN** a release named for the tag holds all four binaries and their checksums, and each binary reports version `0.1.0`

#### Scenario: Failing tests
- **WHEN** the tests fail on a tagged commit
- **THEN** no release is published

### Requirement: Verified installation script
`install.sh` MUST run with a POSIX shell, detect the platform, download the matching binary, and verify its SHA-256 digest before installing. It MUST install to `~/.local/bin` by default without elevated privileges, accept `DEVDOCS_VERSION` and `DEVDOCS_INSTALL_DIR` overrides, and replace an existing binary or symlink atomically without writing through a symlink. A missing checksum entry, a missing hashing tool, a download failure, or an unsupported platform MUST stop installation with a nonzero exit and leave the target directory without new files. A truncated script MUST NOT perform a partial installation.

#### Scenario: Install the latest release
- **WHEN** a user pipes the script into `sh` on a supported platform
- **THEN** the latest release binary is verified and installed, its version is printed, and the user is told if the directory is missing from `PATH`

#### Scenario: Corrupted or incomplete download
- **WHEN** the binary does not match its checksum or `checksums.txt` has no entry for it
- **THEN** installation stops with a nonzero exit and no binary is installed

#### Scenario: Upgrade over a symlink
- **WHEN** the destination is a symlink, such as one to a local development build
- **THEN** the symlink is replaced by the release binary and its former target is unchanged

### Requirement: Post-release installation check
After publishing, a release workflow job MUST run `install.sh` for the new tag on each supported platform and confirm the installed binary reports the tagged version.

#### Scenario: Verify a new release
- **WHEN** a release has been published
- **THEN** every supported platform installs it and reports the tagged version, or the workflow fails visibly
