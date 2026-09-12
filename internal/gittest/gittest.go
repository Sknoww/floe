// Package gittest makes throwaway git repositories for tests. They are real
// repositories; nothing is mocked.
package gittest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Main runs a package's tests with git isolated from the machine: no system or
// global gitconfig can change what a test sees. Each repository declares its
// own identity and initial branch instead.
func Main(m *testing.M) {
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Exit(m.Run())
}

// Init makes an empty repository on branch main, with no commits. extra is
// passed to git init, e.g. "--object-format=sha256".
func Init(t testing.TB, extra ...string) string {
	t.Helper()
	dir := t.TempDir()
	Git(t, dir, append([]string{"init", "--quiet", "--initial-branch=main"}, extra...)...)
	Git(t, dir, "config", "user.name", "Test")
	Git(t, dir, "config", "user.email", "test@example.com")
	return dir
}

// Git runs git in dir and returns its stdout with surrounding whitespace
// trimmed, failing the test if git fails.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_EDITOR=true", "GIT_MERGE_AUTOEDIT=no")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

// Write writes a repo-relative file, creating its directories.
func Write(t testing.TB, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Read reads a repo-relative file.
func Read(t testing.TB, dir, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Exists reports whether a repo-relative path exists.
func Exists(t testing.TB, dir, path string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(path)))
	return err == nil
}

// Remove deletes a repo-relative file.
func Remove(t testing.TB, dir, path string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
		t.Fatal(err)
	}
}

// Commit stages everything and commits it with msg, empty commits and empty
// messages allowed, and returns the new commit's id.
func Commit(t testing.TB, dir, msg string) string {
	t.Helper()
	Git(t, dir, "add", "--all")
	Git(t, dir, "commit", "--quiet", "--allow-empty", "--allow-empty-message", "-m", msg)
	return Git(t, dir, "rev-parse", "HEAD")
}
