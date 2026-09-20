// Package pair owns what floe remembers about a source/target pair: the paths
// that must never cross, the content guard, and when the pair was last opened.
// It lives outside both repositories, one JSON file per pair in the user's
// config directory.
package pair

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// Root is floe's config directory: $XDG_CONFIG_HOME/floe, or ~/.config/floe
// when that is unset or not absolute. It is drift's convention rather than
// os.UserConfigDir, which on macOS is ~/Library/Application Support — away from
// the other terminal tools' config, where a user editing it looks.
func Root() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); filepath.IsAbs(dir) {
		return filepath.Join(dir, "floe"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "floe"), nil
}

// Config is one pair's file.
type Config struct {
	// Source and Target are both repositories' absolute, symlink-resolved
	// top-level paths, as git.Open resolves them. They identify the pair.
	Source string `json:"source"`
	Target string `json:"target"`
	// Exclude holds doublestar globs matched against whole repo-relative
	// paths: a path matching one is never transferred.
	Exclude []string `json:"exclude"`
	// Guard holds Go (RE2) regular expressions: a transfer whose added lines
	// or message match one is refused.
	Guard      []string  `json:"guard"`
	LastOpened time.Time `json:"lastOpened,omitzero"`
}

// ID names a pair in floe's URLs and file names: both directories' names,
// readable in a listing, then the first 8 hex digits of
// sha256(source + "\0" + target), so same-named repositories in different
// places never share one.
func ID(source, target string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + target))
	return filepath.Base(source) + "--" + filepath.Base(target) + "-" + hex.EncodeToString(sum[:4])
}

// FileName is a pair's file name in pairs/: its ID, then ".json".
func FileName(source, target string) string {
	return ID(source, target) + ".json"
}

// Path is where a pair's file lives under root.
func Path(root, source, target string) string {
	return filepath.Join(root, "pairs", FileName(source, target))
}

// Load reads a pair's file. It is strict: an unknown field, a missing
// repository path, or a pattern that is invalid or can never match is an error
// naming the file and the field. A setting that silently didn't apply is
// indistinguishable on screen from one that did.
func Load(path string) (*Config, error) {
	var c Config
	if err := readStrict(path, &c); err != nil {
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// LoadID reads the pair an ID names. The ID must be a plain file name, and the
// file must name the pair the ID is made from, so a renamed or copied file
// cannot stand in for another pair. An ID that names no pair is an error
// matching fs.ErrNotExist.
func LoadID(root, id string) (*Config, error) {
	if id == "" || id != filepath.Base(id) || strings.ContainsRune(id, 0) {
		return nil, fmt.Errorf("%q is not a pair id: %w", id, fs.ErrNotExist)
	}
	path := filepath.Join(root, "pairs", id+".json")
	c, err := Load(path)
	if err != nil {
		return nil, err
	}
	if got := ID(c.Source, c.Target); got != id {
		return nil, fmt.Errorf("%s: names the pair %s → %s, whose file is %s.json", path, c.Source, c.Target, got)
	}
	return c, nil
}

// readStrict decodes the JSON object in the file at path into v, refusing an
// unknown field or anything after the object.
func readStrict(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("%s: unexpected data after the JSON object", path)
	}
	return nil
}

// Save writes the file atomically. It refuses a config Load would refuse.
func (c *Config) Save(path string) error {
	if err := c.validate(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	out := *c
	// Empty lists are written out, so a hand edit finds the field to fill in.
	if out.Exclude == nil {
		out.Exclude = []string{}
	}
	if out.Guard == nil {
		out.Guard = []string{}
	}
	return writeAtomic(path, out)
}

// writeAtomic writes v as indented JSON through a temp file and a rename, so a
// crash or a full disk never leaves half a file.
func writeAtomic(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Guard patterns are regular expressions: "<" stays "<", not "<".
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename succeeds
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func (c *Config) validate() error {
	for _, f := range []struct{ name, path string }{{"source", c.Source}, {"target", c.Target}} {
		if f.path == "" {
			return fmt.Errorf("%q is missing", f.name)
		}
		if !filepath.IsAbs(f.path) {
			return fmt.Errorf("%q: %q is not an absolute path", f.name, f.path)
		}
	}
	if _, err := c.Excluded(); err != nil {
		return err
	}
	_, err := c.CompiledGuard()
	return err
}

// Excluded checks Exclude and returns the predicate a transfer asks.
func (c *Config) Excluded() (func(path string) bool, error) {
	for i, p := range c.Exclude {
		if err := CheckExclude(p); err != nil {
			return nil, fmt.Errorf("exclude[%d]: %w", i, err)
		}
	}
	patterns := slices.Clone(c.Exclude)
	return func(path string) bool {
		for _, p := range patterns {
			if doublestar.MatchUnvalidated(p, path) {
				return true
			}
		}
		return false
	}, nil
}

// CompiledGuard compiles Guard, in order.
func (c *Config) CompiledGuard() ([]*regexp.Regexp, error) {
	guard := make([]*regexp.Regexp, len(c.Guard))
	for i, p := range c.Guard {
		re, err := CompileGuard(p)
		if err != nil {
			return nil, fmt.Errorf("guard[%d]: %w", i, err)
		}
		guard[i] = re
	}
	return guard, nil
}

// ExcludeError is an exclusion pattern that is malformed or can never match,
// with the pattern it most likely meant when there is one.
type ExcludeError struct {
	Reason     string
	Suggestion string // "" when there is none
}

func (e *ExcludeError) Error() string { return e.Reason }

// CheckExclude refuses an exclusion pattern that is malformed or can never
// match a path git reports, with an *ExcludeError. Patterns match whole
// repo-relative paths from the top level: "README.md" is only the top-level
// README, "docs" is a file named docs, and "docs/**" is everything under the
// docs directory.
func CheckExclude(pattern string) error {
	switch {
	case strings.Trim(pattern, "/") == "":
		return &ExcludeError{Reason: "the pattern is empty"}
	case strings.HasPrefix(pattern, "/"):
		return deadPattern(pattern, strings.TrimLeft(pattern, "/"),
			"%q can never match: patterns are relative to the repository's top level, so write %q")
	case strings.HasSuffix(pattern, "/"):
		return deadPattern(pattern, strings.TrimRight(pattern, "/")+"/**",
			"%q can never match a file: to exclude everything under a directory, write %q")
	}
	for _, seg := range strings.Split(pattern, "/") {
		switch seg {
		case "":
			return &ExcludeError{Reason: fmt.Sprintf("%q can never match: it has an empty path segment", pattern)}
		case ".", "..":
			return &ExcludeError{Reason: fmt.Sprintf("%q can never match: repo-relative paths have no %q segments", pattern, seg)}
		}
	}
	if !doublestar.ValidatePattern(pattern) {
		return &ExcludeError{Reason: fmt.Sprintf("%q is not a valid glob", pattern)}
	}
	return nil
}

// deadPattern refuses pattern, suggesting fix — or what fix is suggested in
// turn, so "/docs/" is offered "docs/**" rather than another dead pattern. A
// fix that fails with nothing to suggest is named but not offered.
func deadPattern(pattern, fix, reason string) error {
	var fixErr *ExcludeError
	if errors.As(CheckExclude(fix), &fixErr) {
		if fixErr.Suggestion == "" {
			return &ExcludeError{Reason: fmt.Sprintf(reason, pattern, fix)}
		}
		fix = fixErr.Suggestion
	}
	return &ExcludeError{Reason: fmt.Sprintf(reason, pattern, fix), Suggestion: fix}
}

// CompileGuard compiles one content guard pattern: a Go (RE2) regular
// expression, case-sensitive unless it starts (?i). An empty pattern would
// refuse every transfer, so it is refused itself.
func CompileGuard(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, errors.New("the pattern is empty and would match every line")
	}
	return regexp.Compile(pattern)
}

// Remember opens a pair's file under root — creating it for a pair not seen
// before — and records now as when it was last opened. source and target are
// the repositories' resolved top-level paths.
func Remember(root, source, target string, now time.Time) (*Config, error) {
	path := Path(root, source, target)
	c, err := Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		c, err = &Config{Source: source, Target: target}, nil
	}
	if err != nil {
		return nil, err
	}
	if c.Source != source || c.Target != target {
		return nil, fmt.Errorf("%s: names the pair %s → %s, not %s → %s", path, c.Source, c.Target, source, target)
	}
	c.LastOpened = now.UTC().Truncate(time.Second)
	if err := c.Save(path); err != nil {
		return nil, err
	}
	return c, nil
}

// Remembered is one pair file found under root.
type Remembered struct {
	Path   string
	Config *Config // nil when Err is set
	Err    error   // the file could not be read, or is invalid
	// Missing lists the pair's repository paths that are no longer
	// directories. Moving a repository orphans its pair, which is shown as
	// missing rather than hidden.
	Missing []string
}

// List reads every pair file under root, most recently opened first. A file
// that fails to load is listed with its error, after the rest, rather than
// hiding them.
func List(root string) ([]Remembered, error) {
	dir := filepath.Join(root, "pairs")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pairs []Remembered
	for _, e := range entries {
		// A save's temp file ends ".tmp…", not ".json".
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r := Remembered{Path: filepath.Join(dir, e.Name())}
		r.Config, r.Err = Load(r.Path)
		if r.Err == nil {
			for _, repo := range []string{r.Config.Source, r.Config.Target} {
				if fi, err := os.Stat(repo); err != nil || !fi.IsDir() {
					r.Missing = append(r.Missing, repo)
				}
			}
		}
		pairs = append(pairs, r)
	}
	// ReadDir sorts by name, which is the order kept among broken files.
	slices.SortStableFunc(pairs, func(a, b Remembered) int {
		switch {
		case a.Err != nil && b.Err != nil:
			return 0
		case a.Err != nil:
			return 1
		case b.Err != nil:
			return -1
		}
		return b.Config.LastOpened.Compare(a.Config.LastOpened)
	})
	return pairs, nil
}
