package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Commit is one commit on the source's first-parent history.
type Commit struct {
	ID          string
	Parents     []string // first parent first; empty for a root commit
	AuthorName  string
	AuthorEmail string
	AuthorTime  time.Time
	Subject     string
}

// commitFields is the number of NUL-separated fields --format writes per commit.
const commitFields = 6

// Commits lists HEAD's first-parent history, newest first. A merge is one entry,
// standing for everything it brought in; the commits on the merged branch are
// not listed. That keeps the history linear, which the position walk depends
// on. A repository with no commits lists none.
func (r *Repo) Commits(ctx context.Context) ([]Commit, error) {
	head, err := r.Head(ctx)
	if err != nil || head == "" {
		return nil, err
	}
	out, err := r.run(ctx, call{args: []string{
		"log", "--first-parent", "--no-show-signature", "-z",
		"--format=%H%x00%P%x00%an%x00%ae%x00%at%x00%s",
		head,
	}})
	if err != nil {
		return nil, err
	}
	// -z ends every record with a NUL, and the fields inside a record are
	// NUL-separated too, so the output is a flat run of fields. None of them
	// can contain a NUL, and an empty subject is still a field.
	fields := nulRecords(out)
	if len(fields)%commitFields != 0 {
		return nil, fmt.Errorf("git log: %d fields is not a whole number of commits", len(fields))
	}
	commits := make([]Commit, 0, len(fields)/commitFields)
	for i := 0; i < len(fields); i += commitFields {
		f := fields[i : i+commitFields]
		secs, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git log: commit %s: author time %q: %w", f[0], f[4], err)
		}
		commits = append(commits, Commit{
			ID:          f[0],
			Parents:     strings.Fields(f[1]),
			AuthorName:  f[2],
			AuthorEmail: f[3],
			AuthorTime:  time.Unix(secs, 0),
			Subject:     f[5],
		})
	}
	return commits, nil
}

// Message is a commit's full message, exactly as recorded. "format:" rather
// than --format: the latter terminates the entry with a newline of its own.
func (r *Repo) Message(ctx context.Context, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	out, err := r.run(ctx, call{args: []string{"log", "-1", "--no-show-signature", "--pretty=format:%B", id}})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// diffBase is what a commit is diffed against: its first parent, or the empty
// tree for a root commit. A merge diffed against its first parent is exactly
// what it brought in.
func (r *Repo) diffBase(ctx context.Context, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	out, err := r.run(ctx, call{args: []string{"rev-parse", "--verify", "-q", id + "^1"}})
	if exitCode(err) == 1 {
		return r.EmptyTree(ctx)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Change is one path a commit changed, as git's raw diff reports it. Renames
// are never detected, so a moved file is a deletion and an addition, each on
// its own path.
type Change struct {
	Path    string
	Status  byte   // 'A' added, 'M' modified, 'D' deleted, 'T' type changed, 'U' unmerged
	OldMode string // "000000" when added
	NewMode string // "000000" when deleted
	OldID   string // all zeros when added
	NewID   string // all zeros when deleted
}

// Changes lists the paths a commit changed against its diff base, in git's
// path order.
func (r *Repo) Changes(ctx context.Context, id string) ([]Change, error) {
	base, err := r.diffBase(ctx, id)
	if err != nil {
		return nil, err
	}
	out, err := r.run(ctx, call{args: []string{
		"diff-tree", "-r", "-z", "--no-renames", "--no-abbrev", "--no-commit-id", base, id,
	}})
	if err != nil {
		return nil, err
	}
	return parseRaw(nulRecords(out))
}

// parseRaw reads -z raw diff output already split on NUL: a metadata token
// (":<old mode> <new mode> <old id> <new id> <status>") followed by a path
// token, repeated. With renames off, every entry has exactly one path.
func parseRaw(tokens []string) ([]Change, error) {
	if len(tokens)%2 != 0 {
		return nil, fmt.Errorf("raw diff: %d tokens is not a whole number of entries", len(tokens))
	}
	changes := make([]Change, 0, len(tokens)/2)
	for i := 0; i < len(tokens); i += 2 {
		c, err := parseRawEntry(tokens[i], tokens[i+1])
		if err != nil {
			return nil, err
		}
		changes = append(changes, c)
	}
	return changes, nil
}

func parseRawEntry(meta, path string) (Change, error) {
	f := strings.Fields(strings.TrimPrefix(strings.TrimLeft(meta, "\n"), ":"))
	if len(f) != 5 || f[4] == "" {
		return Change{}, fmt.Errorf("raw diff: malformed entry %q", meta)
	}
	return Change{
		Path:    path,
		Status:  f[4][0],
		OldMode: f[0],
		NewMode: f[1],
		OldID:   f[2],
		NewID:   f[3],
	}, nil
}

// LineCount is how many lines a commit added to one file and deleted from it.
// A binary file has no lines to count.
type LineCount struct {
	Added, Deleted int
	Binary         bool
}

// LineCounts reports, by path, the lines a commit added and deleted against its
// diff base.
func (r *Repo) LineCounts(ctx context.Context, id string) (map[string]LineCount, error) {
	base, err := r.diffBase(ctx, id)
	if err != nil {
		return nil, err
	}
	out, err := r.run(ctx, call{args: []string{
		"diff-tree", "-r", "-z", "--numstat", "--no-renames", "--no-commit-id", base, id,
	}})
	if err != nil {
		return nil, err
	}
	counts := map[string]LineCount{}
	for _, rec := range nulRecords(out) {
		// "<added>\t<deleted>\t<path>", with "-" for both counts of a binary
		// file. With renames off there is one path, taken whole.
		f := strings.SplitN(rec, "\t", 3)
		if len(f) != 3 {
			return nil, fmt.Errorf("numstat: malformed entry %q", rec)
		}
		if f[0] == "-" && f[1] == "-" {
			counts[f[2]] = LineCount{Binary: true}
			continue
		}
		added, errAdded := strconv.Atoi(f[0])
		deleted, errDeleted := strconv.Atoi(f[1])
		if errAdded != nil || errDeleted != nil {
			return nil, fmt.Errorf("numstat: malformed entry %q", rec)
		}
		counts[f[2]] = LineCount{Added: added, Deleted: deleted}
	}
	return counts, nil
}

// diffArgs starts a `git diff` with every flag user configuration could
// otherwise change pinned: a diff.noprefix, diff.external or color.diff=always
// in someone's gitconfig would produce a patch apply cannot read, or reads
// wrongly.
func diffArgs(extra ...string) []string {
	return append([]string{
		"diff", "--no-renames",
		"--no-color", "--no-ext-diff", "--no-textconv", "--no-relative",
		"--src-prefix=a/", "--dst-prefix=b/",
	}, extra...)
}

// Patch is the commit's change against its diff base as a binary-safe patch
// `git apply` accepts, with the given repo-relative paths left out. Each path is
// excluded literally: matching patterns to paths is the caller's job, done once,
// so the file list, the patch and the position walk cannot disagree.
//
// --full-index writes whole blob ids on the index lines; they are the ids the
// preimage import supplies.
func (r *Repo) Patch(ctx context.Context, id string, exclude []string) ([]byte, error) {
	base, err := r.diffBase(ctx, id)
	if err != nil {
		return nil, err
	}
	args := append(diffArgs("--binary", "--full-index"), base, id, "--", ".")
	for _, p := range exclude {
		args = append(args, ":(exclude,literal)"+p)
	}
	return r.run(ctx, call{args: args})
}

// FileDiff is the commit's change to one path against its diff base, to be read
// rather than applied: a binary file is reported as one, not encoded.
func (r *Repo) FileDiff(ctx context.Context, id, path string) ([]byte, error) {
	base, err := r.diffBase(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.run(ctx, call{args: append(diffArgs(), base, id, "--", ":(literal)"+path)})
}
