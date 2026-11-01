## Purpose

Show whether private context is locally changed, safely isolated, and published without performing network activity or hiding ignored documents.

## ADDED Requirements

### Requirement: Offline comprehensive status
`status` MUST operate offline and report identity, docs path, branch, pinned remote, staged/unstaged/untracked/ignored paths, outstanding Git operations, and link health. It MUST distinguish unpublished projects from published ones and label ahead/behind information as based on potentially stale fetched state. Unsafe tracking or exclusions MUST produce nonzero status with actionable paths. When metadata cannot be read during a Git operation, status MUST report that operation rather than the metadata error.

#### Scenario: Unpublished or offline dirty workspace (A2, A8, A13)
- **WHEN** status runs without network access on an unpublished or locally modified workspace containing ignored documents
- **THEN** it reports publication state and changed/ignored paths without fetching or claiming all files are backed up

#### Scenario: Stale or changed links (A16)
- **WHEN** a link is missing, changed, or locally owned after its declaration was removed
- **THEN** status identifies that state without repairing or deleting it implicitly

#### Scenario: Existing unsafe workspace (A12, A15)
- **WHEN** managed paths become tracked, exclusions are overridden, or a Git operation remains in progress
- **THEN** status reports the exact safety/operation issue and does not silently present the workspace as healthy
