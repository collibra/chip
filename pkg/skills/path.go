package skills

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// resolveSkillsDir turns a user-supplied path into an absolute path to an
// existing directory. It expands `~` to the current user's home and
// `~name` to that user's home (Unix shell convention; Windows users
// without an /etc/passwd-equivalent get an error from user.Lookup).
// Returns a wrapped error if the path is missing, unreadable, or not a
// directory.
func resolveSkillsDir(p string) (string, error) {
	expanded, err := expandTilde(p)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("absolutize skills dir %q: %w", p, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat skills dir %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("skills dir %q is not a directory", abs)
	}
	return abs, nil
}

func expandTilde(p string) (string, error) {
	if p == "" || p[0] != '~' {
		return p, nil
	}
	// Split off the user component, if any: "~bob/x" → "bob", "/x".
	// Bare "~" or "~/..." use the current user.
	slash := strings.IndexByte(p, filepath.Separator)
	if filepath.Separator != '/' {
		// On Windows accept '/' as a separator inside config-supplied
		// paths too, since users routinely write '~/foo' in YAML.
		if i := strings.IndexByte(p, '/'); i >= 0 && (slash < 0 || i < slash) {
			slash = i
		}
	}
	var userPart, rest string
	if slash < 0 {
		userPart = p[1:]
	} else {
		userPart = p[1:slash]
		rest = p[slash+1:]
	}

	var home string
	if userPart == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		home = h
	} else {
		u, err := user.Lookup(userPart)
		if err != nil {
			return "", fmt.Errorf("look up user %q: %w", userPart, err)
		}
		home = u.HomeDir
	}
	if rest == "" {
		return home, nil
	}
	return filepath.Join(home, rest), nil
}
