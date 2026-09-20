package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Open resolves dir to its repository's top-level directory, absolute and with
// symlinks resolved, so the same repository reached two ways is one Repo. A bare
// repository has no working tree to transfer into and is refused.
func Open(ctx context.Context, dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(abs); err != nil {
		return nil, err
	} else if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}
	probe := &Repo{Dir: abs}
	// Asked separately: in a bare repository --show-toplevel fails outright,
	// and "not a working tree" would hide why.
	out, err := probe.run(ctx, call{args: []string{"rev-parse", "--is-bare-repository"}})
	if err != nil {
		return nil, fmt.Errorf("%s is not a git repository: %w", abs, err)
	}
	if strings.TrimSpace(string(out)) == "true" {
		return nil, fmt.Errorf("%s is a bare repository; floe needs a working tree", abs)
	}
	out, err = probe.run(ctx, call{args: []string{"rev-parse", "--show-toplevel"}})
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a working tree: %w", abs, err)
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, err
	}
	return &Repo{Dir: top}, nil
}

// ObjectFormat is the repository's hash algorithm: "sha1" or "sha256". Blob ids
// are the whole transfer mechanism, so two repositories that disagree share none.
func (r *Repo) ObjectFormat(ctx context.Context) (string, error) {
	out, err := r.run(ctx, call{args: []string{"rev-parse", "--show-object-format"}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Head is the commit HEAD points at, or "" when the current branch has no
// commits yet. An empty target is a supported target: its first transfer is
// the one that gives it a history.
func (r *Repo) Head(ctx context.Context) (string, error) {
	out, err := r.run(ctx, call{args: []string{"rev-parse", "--verify", "-q", "HEAD^{commit}"}})
	if exitCode(err) == 1 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Branch is the branch HEAD is on, also before its first commit, or "" when
// HEAD is detached.
func (r *Repo) Branch(ctx context.Context) (string, error) {
	out, err := r.run(ctx, call{args: []string{"symbolic-ref", "--short", "-q", "HEAD"}})
	if exitCode(err) == 1 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// EmptyTree is the id of the empty tree in this repository's object format,
// asked of git rather than hardcoded so SHA-256 repositories work. It is what a
// root commit is diffed against. Nothing is written.
func (r *Repo) EmptyTree(ctx context.Context) (string, error) {
	out, err := r.run(ctx, call{args: []string{"hash-object", "-t", "tree", os.DevNull}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

var objectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// checkID refuses anything but a full object id. Ids reach this package from
// the API, and an id is placed on git's command line: a value starting with a
// dash would be read as an option.
func checkID(id string) error {
	if !objectID.MatchString(id) {
		return fmt.Errorf("%q is not a full object id", id)
	}
	return nil
}

// IsObjectID reports whether id is a full object id, as every id reaching this
// package must be.
func IsObjectID(id string) bool { return objectID.MatchString(id) }

// isZeroID reports whether id is git's all-zeros id: the old side of an added
// file, the new side of a deleted one.
func isZeroID(id string) bool {
	return strings.Trim(id, "0") == ""
}

// Version is a git release number.
type Version struct {
	Major, Minor, Patch int
}

// MinVersion is the oldest git floe runs against. 2.32.0 is the release in
// which `git apply --3way` tries the 3-way merge first and accepts --cached
// alongside it; before it, a preview on a temporary index could not be run at
// all, and an apply fell back to a straight patch first — so what a preview
// showed and what an apply did could differ.
var MinVersion = Version{2, 32, 0}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Less reports whether v is an older release than o.
func (v Version) Less(o Version) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

var versionLine = regexp.MustCompile(`^git version (\d+)\.(\d+)(?:\.(\d+))?`)

// parseVersion reads `git version` output, which vendors decorate:
// "git version 2.50.1 (Apple Git-155)", "git version 2.45.2.windows.1".
func parseVersion(out string) (Version, error) {
	m := versionLine.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return Version{}, fmt.Errorf("unrecognised git version output %q", strings.TrimSpace(out))
	}
	var v Version
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		v.Patch, _ = strconv.Atoi(m[3])
	}
	return v, nil
}

// InstalledVersion is the version of the git on PATH.
func InstalledVersion(ctx context.Context) (Version, error) {
	out, err := (&Repo{}).run(ctx, call{args: []string{"version"}})
	if err != nil {
		return Version{}, err
	}
	return parseVersion(string(out))
}

// CheckVersion refuses a git older than MinVersion, naming both versions.
func CheckVersion(ctx context.Context) error {
	v, err := InstalledVersion(ctx)
	if err != nil {
		return err
	}
	if v.Less(MinVersion) {
		return fmt.Errorf("git %s is installed; floe needs git %s or newer", v, MinVersion)
	}
	return nil
}
