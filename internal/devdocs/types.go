package devdocs

// SchemaVersionCurrent is the current metadata schema version.
const SchemaVersionCurrent = 1

// Config represents global devdocs configuration.
type Config struct {
	DefaultRemote string `json:"default_remote,omitempty"`
}

// LinkDeclaration defines a link mapping in .devdocs.json.
// Source is relative to devdocs/, destination is relative to code root.
type LinkDeclaration struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// Metadata represents the versioned .devdocs.json project file.
type Metadata struct {
	SchemaVersion int               `json:"schema_version"`
	ProjectID     string            `json:"project_id"`
	Branch        string            `json:"branch"`
	Name          string            `json:"name"`
	CreatedAt     string            `json:"created_at"`
	Links         []LinkDeclaration `json:"links"`
}

// InstalledLink tracks an installed symlink's ownership in devdocs-state.json.
type InstalledLink struct {
	Destination string `json:"destination"`
	Target      string `json:"target"`
}

// LocalState represents machine-local uncommitted state stored in docs .git/devdocs-state.json.
type LocalState struct {
	ProjectID      string          `json:"project_id"`
	Branch         string          `json:"branch"`
	Remote         string          `json:"remote"`
	Published      bool            `json:"published"`
	InstalledLinks []InstalledLink `json:"installed_links"`
}

// Workspace holds resolved paths for the code repository and its nested devdocs workspace.
type Workspace struct {
	CodeRoot       string // Canonical absolute path to code repository root
	DocsDir        string // Path to devdocs directory (<CodeRoot>/devdocs)
	GitDir         string // Path to code repository .git (dir or worktree gitdir)
	CommonGitDir   string // Path to common git directory (for shared excludes and locks)
	InsideDocsRepo bool   // True if invocation originated from inside devdocs
}
