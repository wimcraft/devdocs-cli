package devdocs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// DefaultConfigPath returns the canonical path to the devdocs global config file.
func DefaultConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); strings.TrimSpace(xdg) != "" {
		return filepath.Join(xdg, "devdocs", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "devdocs", "config.json"), nil
}

// LoadConfig reads and parses the global configuration file.
// If the file does not exist, an empty Config is returned with no error.
func LoadConfig() (*Config, string, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, path, nil
		}
		return nil, path, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, path, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	if cfg.DefaultRemote != "" {
		if err := ValidateRemoteURL(cfg.DefaultRemote); err != nil {
			return nil, path, fmt.Errorf("invalid default_remote in %s: %w", path, err)
		}
	}

	return &cfg, path, nil
}

// SaveConfig writes the configuration atomically to the config file path.
func SaveConfig(cfg *Config) (string, error) {
	if cfg.DefaultRemote != "" {
		if err := ValidateRemoteURL(cfg.DefaultRemote); err != nil {
			return "", err
		}
	}

	path, err := DefaultConfigPath()
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serializing config: %w", err)
	}
	data = append(data, '\n')

	f, err := os.CreateTemp(dir, "config.json.tmp.*")
	if err != nil {
		return "", fmt.Errorf("creating temp config file: %w", err)
	}
	tmpPath := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := f.Write(data); err != nil {
		return "", fmt.Errorf("writing config data: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("syncing config file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing config file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("renaming config file: %w", err)
	}

	return path, nil
}

// ValidateRemoteURL checks that a remote URL is a valid, safe Git remote.
// Rejects option-like strings, embedded credentials in HTTP/HTTPS, and malformed URLs.
func ValidateRemoteURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("remote URL cannot be empty")
	}
	if strings.HasPrefix(trimmed, "-") {
		return errors.New("remote URL cannot start with a dash")
	}
	if strings.ContainsAny(trimmed, "\x00\r\n") || (strings.Contains(trimmed, "::") && !strings.Contains(trimmed, "://")) {
		return errors.New("remote URL contains unsupported transport syntax or control characters")
	}

	// Reject embedded credentials in HTTP/HTTPS
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return errors.New("invalid HTTP/HTTPS remote URL")
		}
		if u.User != nil {
			return errors.New("remote URL must not contain embedded username or password credentials")
		}
		if u.Host == "" {
			return errors.New("remote URL has empty host")
		}
		return nil
	}

	// SSH URL format: ssh://user@host/path or scp-like user@host:path
	if strings.HasPrefix(lower, "ssh://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return errors.New("invalid SSH remote URL")
		}
		if pass, hasPass := u.User.Password(); hasPass || pass != "" {
			return errors.New("SSH remote URL must not contain embedded password")
		}
		return nil
	}

	// file:// URLs
	if strings.HasPrefix(lower, "file://") {
		return nil
	}

	// Local filesystem paths or SCP-like syntax (e.g. git@github.com:owner/repo.git)
	if strings.Contains(trimmed, "@") && strings.Contains(trimmed, ":") {
		// SCP-style SSH url: user@host:path
		// Ensure no embedded password before the '@'
		userPart := trimmed[:strings.Index(trimmed, "@")]
		if strings.Contains(userPart, ":") {
			return errors.New("SCP-style remote URL must not contain embedded password")
		}
		return nil
	}

	// Could be a local filesystem directory/path
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") {
		return nil
	}

	// Check for git:// or other known schemes
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return errors.New("invalid remote URL")
		}
		if u.User != nil {
			return errors.New("remote URL must not contain embedded credentials")
		}
		return nil
	}

	return nil
}

var remoteUserinfoPattern = regexp.MustCompile(`(?i)((?:https?|ssh)://)[^/\s]*@`)

// RedactRemoteURL removes URL userinfo from a URL or a larger Git diagnostic.
func RedactRemoteURL(raw string) string {
	return remoteUserinfoPattern.ReplaceAllString(strings.TrimSpace(raw), "${1}[redacted]@")
}

// TokenizeCommandLine splits a command string into arguments respecting quotes without a shell.
func TokenizeCommandLine(cmd string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for _, r := range cmd {

		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' && !inSingle {
			escaped = true
			continue
		}

		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}

		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if unicode.IsSpace(r) && !inSingle && !inDouble {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if inSingle || inDouble {
		return nil, errors.New("unmatched quote in command line")
	}
	if escaped {
		current.WriteByte('\\')
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}

// EditConfigFile launches the user's preferred editor ($VISUAL then $EDITOR) to edit the config file.
func EditConfigFile(in io.Reader, out, errOut io.Writer) error {
	editorEnv := os.Getenv("VISUAL")
	if strings.TrimSpace(editorEnv) == "" {
		editorEnv = os.Getenv("EDITOR")
	}
	if strings.TrimSpace(editorEnv) == "" {
		return errors.New("neither VISUAL nor EDITOR environment variable is set; set one or use 'devdocs config --remote <url>'")
	}

	cfgPath, err := DefaultConfigPath()
	if err != nil {
		return err
	}

	// If config file doesn't exist yet, create a default template so the editor has something to open.
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		if _, err := SaveConfig(&Config{}); err != nil {
			return fmt.Errorf("initializing config file before editing: %w", err)
		}
	}

	tokens, err := TokenizeCommandLine(editorEnv)
	if err != nil {
		return fmt.Errorf("parsing editor command %q: %w", editorEnv, err)
	}
	if len(tokens) == 0 {
		return errors.New("empty editor command")
	}

	args := append(tokens[1:], cfgPath)
	cmd := exec.Command(tokens[0], args...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q exited with error: %w", tokens[0], err)
	}

	// Validate after editing
	if _, _, err := LoadConfig(); err != nil {
		return fmt.Errorf("config file is invalid after editing: %w", err)
	}

	return nil
}
