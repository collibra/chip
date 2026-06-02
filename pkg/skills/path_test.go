package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSkillsDir_absolutePathPassesThrough(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveSkillsDir(dir)
	if err != nil {
		t.Fatalf("resolveSkillsDir: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestResolveSkillsDir_relativePathIsAbsolutized(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	sub := "subdir"
	if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := resolveSkillsDir(sub)
	if err != nil {
		t.Fatalf("resolveSkillsDir: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
	if !strings.HasSuffix(got, sub) {
		t.Errorf("expected suffix %q, got %q", sub, got)
	}
}

func TestResolveSkillsDir_tildeExpandsToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// On macOS os.UserHomeDir consults $HOME first.
	if _, err := os.UserHomeDir(); err != nil {
		t.Skipf("UserHomeDir unavailable: %v", err)
	}
	sub := "my-skills"
	if err := os.Mkdir(filepath.Join(home, sub), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := resolveSkillsDir("~/" + sub)
	if err != nil {
		t.Fatalf("resolveSkillsDir: %v", err)
	}
	want := filepath.Join(home, sub)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSkillsDir_bareTildeExpandsToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := resolveSkillsDir("~")
	if err != nil {
		t.Fatalf("resolveSkillsDir: %v", err)
	}
	if got != home {
		t.Errorf("got %q, want %q", got, home)
	}
}

func TestResolveSkillsDir_missingPathErrors(t *testing.T) {
	_, err := resolveSkillsDir("/definitely/does/not/exist/skills-xyzzy")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestResolveSkillsDir_fileNotDirErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := resolveSkillsDir(file)
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error should mention not-a-directory, got: %v", err)
	}
}
