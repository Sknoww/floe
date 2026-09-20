package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// nulRecords splits NUL-terminated output into its records.
func nulRecords(out []byte) []string {
	s := strings.TrimSuffix(string(out), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// FileStatus is a tracked path with changes, as `git status` reports it.
type FileStatus struct {
	Path string
	// Status is one letter: 'U' for a conflict, else the staged change's ('M',
	// 'A', 'D', 'T') when there is one, else the working tree's.
	Status byte
}

// Status lists the tracked paths that have changes, staged or not, in path
// order. Untracked files are not counted: they cannot mix into a commit, and a
// patch that would overwrite one is refused by git apply itself.
func (r *Repo) Status(ctx context.Context) ([]FileStatus, error) {
	out, err := r.run(ctx, call{args: []string{
		"status", "--porcelain", "-z", "--untracked-files=no", "--no-renames",
	}})
	if err != nil {
		return nil, err
	}
	var files []FileStatus
	for _, rec := range nulRecords(out) {
		// "XY <path>"; with renames off there is never a second path.
		if len(rec) < 4 {
			return nil, fmt.Errorf("status: malformed entry %q", rec)
		}
		files = append(files, FileStatus{Path: rec[3:], Status: statusLetter(rec[0], rec[1])})
	}
	return files, nil
}

// statusLetter reduces a porcelain XY code to one letter. The unmerged codes
// are DD, AU, UD, UA, DU, AA and UU.
func statusLetter(x, y byte) byte {
	switch {
	case x == 'U' || y == 'U' || (x == y && (x == 'A' || x == 'D')):
		return 'U'
	case x != ' ':
		return x
	}
	return y
}

// Dirty lists the paths Status reports.
func (r *Repo) Dirty(ctx context.Context) ([]string, error) {
	files, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths, nil
}

// Unmerged lists the paths with unmerged index entries — the files a conflict
// left markers in — once each, in path order.
func (r *Repo) Unmerged(ctx context.Context) ([]string, error) {
	return r.unmerged(ctx, nil)
}

func (r *Repo) unmerged(ctx context.Context, env []string) ([]string, error) {
	out, err := r.run(ctx, call{args: []string{"ls-files", "-u", "-z"}, env: env})
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, rec := range nulRecords(out) {
		// "<mode> <id> <stage>\t<path>", one record per stage.
		_, path, ok := strings.Cut(rec, "\t")
		if !ok {
			return nil, fmt.Errorf("ls-files: malformed entry %q", rec)
		}
		paths = append(paths, path)
	}
	return compactPaths(paths), nil
}

// compactPaths drops adjacent repeats: an unmerged path is listed once per
// stage, and the stages of one path are adjacent in index order.
func compactPaths(paths []string) []string {
	var out []string
	for _, p := range paths {
		if len(out) == 0 || out[len(out)-1] != p {
			out = append(out, p)
		}
	}
	return out
}

// MissingObjects returns the ids in ids that r's object store lacks, in order.
func (r *Repo) MissingObjects(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	for _, id := range ids {
		if err := checkID(id); err != nil {
			return nil, err
		}
	}
	out, err := r.run(ctx, call{
		args:  []string{"cat-file", "--batch-check"},
		stdin: []byte(strings.Join(ids, "\n") + "\n"),
	})
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if id, ok := strings.CutSuffix(line, " missing"); ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// gitlinkMode is the mode of a submodule entry.
const gitlinkMode = "160000"

// Preimages lists the blob ids a patch of changes merges against — the old side
// of every change that has one — once each. An added file has none, and a
// submodule's old side is a commit in another repository, not a blob here.
func Preimages(changes []Change) []string {
	seen := map[string]bool{}
	var ids []string
	for _, c := range changes {
		if isZeroID(c.OldID) || c.OldMode == gitlinkMode || seen[c.OldID] {
			continue
		}
		seen[c.OldID] = true
		ids = append(ids, c.OldID)
	}
	return ids
}

// ImportBlobs copies every blob in ids that r lacks from src into r's object
// store, and returns the ids it copied.
//
// `git apply --3way` merges against a file's preimage blob. A target whose file
// differs from the source's preimage does not have that blob, and without it git
// falls back to a straight patch that rejects a conflicting edit outright
// instead of writing conflict markers. The copies are unreachable: invisible to
// status and history, and removed by git's garbage collection.
//
// Each blob is read whole before it is written, so a failed read can never
// store a truncated blob. Content from stdin is hashed without the target's
// clean filters or line-ending conversion unless a --path is given; --no-filters
// pins that rather than relying on it, and the id the blob lands under is
// checked regardless.
func (r *Repo) ImportBlobs(ctx context.Context, src *Repo, ids []string) ([]string, error) {
	missing, err := r.MissingObjects(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range missing {
		blob, err := src.run(ctx, call{args: []string{"cat-file", "blob", id}})
		if err != nil {
			return nil, err
		}
		out, err := r.run(ctx, call{
			args:  []string{"hash-object", "-w", "--no-filters", "--stdin"},
			stdin: blob,
			write: true,
		})
		if err != nil {
			return nil, err
		}
		if got := strings.TrimSpace(string(out)); got != id {
			return nil, fmt.Errorf("importing blob %s: stored as %s", id, got)
		}
	}
	return missing, nil
}

// ApplyResult is what applying a patch did, or — from Preview — would do.
type ApplyResult struct {
	// Files is every path the result changes against HEAD. A conflicted path
	// has status 'U'.
	Files []Change
	// Conflicts lists the paths left with conflict markers.
	Conflicts []string
	// Landed is filled by Preview only: for each text file, the lines the
	// result adds to the target's own version of it, numbered as they stand in
	// the result. That is every line the transfer brings in, including what a
	// 3-way merge takes from the source beyond the patch's added lines — both
	// sides of a conflict, and whatever a merge driver keeps.
	Landed map[string][]Line
	// Conflicted is filled by Preview only: each conflicted text file as the
	// apply will write it, conflict markers included.
	Conflicted map[string]string
}

// ApplyError is a patch git refused. Nothing was written: git apply is all or
// nothing, so one file that cannot be applied stops every other file too, even
// the ones that would have merged.
type ApplyError struct {
	Stderr string
}

func (e *ApplyError) Error() string {
	return "git apply refused the patch: " + e.Stderr
}

// applyArgs pins the flags user configuration could otherwise change:
// apply.whitespace=error would refuse a commit over a trailing space, and
// apply.ignoreWhitespace would loosen how context is matched. Content crosses
// verbatim.
func applyArgs(where string) []string {
	return []string{"apply", where, "--3way", "--whitespace=nowarn", "--no-ignore-whitespace"}
}

// Preview applies patch to a temporary index built from head ("" for a
// repository with no commits) and reports the result, with the lines it brings
// into each file. Neither the real index nor the working tree is touched.
//
// `git apply --check --3way` looks like the tool for this and is not: it never
// attempts the 3-way merge, so it cannot tell a file that would conflict from
// one that would be refused.
func (r *Repo) Preview(ctx context.Context, head string, patch []byte) (*ApplyResult, error) {
	base, err := r.baseTree(ctx, head)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "floe-preview-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	// A path in a fresh directory: git creates the index file itself.
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index")}
	if _, err := r.run(ctx, call{args: []string{"read-tree", base}, env: env, write: true}); err != nil {
		return nil, err
	}
	if err := r.refreshPaths(ctx, env, patch); err != nil {
		return nil, err
	}
	_, applyErr := r.run(ctx, call{args: applyArgs("--cached"), stdin: patch, env: env, write: true})
	res, err := r.applied(ctx, base, env, applyErr)
	if err != nil {
		return nil, err
	}
	if res.Landed, res.Conflicted, err = r.landed(ctx, env, dir, res.Files); err != nil {
		return nil, err
	}
	return res, nil
}

// Apply applies patch to the real index and working tree with a 3-way merge. A
// clean result is staged; a conflict leaves markers in the working tree and
// unmerged entries in the index. Nothing is committed.
func (r *Repo) Apply(ctx context.Context, patch []byte) (*ApplyResult, error) {
	head, err := r.Head(ctx)
	if err != nil {
		return nil, err
	}
	base, err := r.baseTree(ctx, head)
	if err != nil {
		return nil, err
	}
	_, applyErr := r.run(ctx, call{args: applyArgs("--index"), stdin: patch, write: true})
	return r.applied(ctx, base, nil, applyErr)
}

// baseTree is what an apply's result is compared against: HEAD, or the empty
// tree in a repository with no commits.
func (r *Repo) baseTree(ctx context.Context, head string) (string, error) {
	if head == "" {
		return r.EmptyTree(ctx)
	}
	return head, checkID(head)
}

// applied classifies an apply. Exit 0 is clean. Exit 1 with unmerged entries is
// a conflict; exit 1 without them is a refusal. Anything else is reported as it
// is. Unmerged entries can only be this apply's: a transfer starts from a clean
// index.
func (r *Repo) applied(ctx context.Context, base string, env []string, applyErr error) (*ApplyResult, error) {
	var ge *Error
	if applyErr != nil && (!errors.As(applyErr, &ge) || ge.Code != 1) {
		return nil, applyErr
	}
	conflicts, err := r.unmerged(ctx, env)
	if err != nil {
		return nil, err
	}
	if applyErr != nil && len(conflicts) == 0 {
		return nil, &ApplyError{Stderr: ge.Stderr}
	}
	out, err := r.run(ctx, call{
		args: []string{"diff-index", "--cached", "-z", "--no-renames", "--no-abbrev", base},
		env:  env,
	})
	if err != nil {
		return nil, err
	}
	files, err := parseRaw(nulRecords(out))
	if err != nil {
		return nil, err
	}
	return &ApplyResult{Files: files, Conflicts: conflicts}, nil
}

// ResetHard discards everything staged and every change to a tracked file,
// returning the index and working tree to HEAD. Untracked files stay.
//
// A repository with no commits has no HEAD to return to, so the index is
// emptied and the files that were in it are deleted — what `reset --hard` does
// to a file that is staged but not in HEAD.
func (r *Repo) ResetHard(ctx context.Context) error {
	head, err := r.Head(ctx)
	if err != nil {
		return err
	}
	if head != "" {
		// "HEAD" rather than the id just read, so a commit made in between is
		// never reset away.
		_, err := r.run(ctx, call{args: []string{"reset", "--hard", "--quiet", "HEAD"}, write: true})
		return err
	}
	out, err := r.run(ctx, call{args: []string{"ls-files", "-z"}})
	if err != nil {
		return err
	}
	paths := compactPaths(nulRecords(out))
	if _, err := r.run(ctx, call{args: []string{"read-tree", "--empty"}, write: true}); err != nil {
		return err
	}
	for _, p := range paths {
		if err := r.removeFile(p); err != nil {
			return err
		}
	}
	return nil
}

// removeFile deletes a repo-relative file, then each parent directory that
// leaves empty, stopping at the top level.
func (r *Repo) removeFile(path string) error {
	full := filepath.Join(r.Dir, filepath.FromSlash(path))
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	top := r.Dir + string(filepath.Separator)
	for dir := filepath.Dir(full); strings.HasPrefix(dir, top); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			break // not empty, or already gone
		}
	}
	return nil
}

// SquashMsgPath is where this repository keeps SQUASH_MSG, asked of git so a
// linked worktree gets its own.
func (r *Repo) SquashMsgPath(ctx context.Context) (string, error) {
	out, err := r.run(ctx, call{args: []string{"rev-parse", "--git-path", "SQUASH_MSG"}})
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Dir, p)
	}
	return p, nil
}

// WriteSquashMsg carries msg to the next commit. With no merge in progress,
// `git commit` takes SQUASH_MSG as its message and deletes the file, and VS
// Code's git extension fills its commit box from it.
func (r *Repo) WriteSquashMsg(ctx context.Context, msg string) error {
	p, err := r.SquashMsgPath(ctx)
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(msg), 0o644)
}

// SquashMsg reads the waiting SQUASH_MSG, reporting whether there is one.
func (r *Repo) SquashMsg(ctx context.Context) (string, bool, error) {
	p, err := r.SquashMsgPath(ctx)
	if err != nil {
		return "", false, err
	}
	msg, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	return string(msg), true, nil
}

// RemoveSquashMsg removes SQUASH_MSG, reporting whether there was one.
func (r *Repo) RemoveSquashMsg(ctx context.Context) (bool, error) {
	p, err := r.SquashMsgPath(ctx)
	if err != nil {
		return false, err
	}
	if err := os.Remove(p); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
