package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Entry is one path in a tree: its mode and object id. Two entries are the same
// file exactly when both match.
type Entry struct {
	Mode string
	ID   string
}

// Tree lists every path in commit's tree, recursively, keyed by repo-relative
// path. An empty commit id stands for an empty repository and lists nothing.
func (r *Repo) Tree(ctx context.Context, commit string) (map[string]Entry, error) {
	tree := map[string]Entry{}
	if commit == "" {
		return tree, nil
	}
	if err := checkID(commit); err != nil {
		return nil, err
	}
	out, err := r.run(ctx, call{args: []string{"ls-tree", "-r", "-z", "--full-tree", commit}})
	if err != nil {
		return nil, err
	}
	for _, rec := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if rec == "" {
			continue
		}
		// "<mode> SP <type> SP <id> TAB <path>"; the path is taken whole, since
		// it may contain spaces or tabs of its own.
		meta, path, ok := strings.Cut(rec, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 {
			return nil, fmt.Errorf("ls-tree: malformed entry %q", rec)
		}
		tree[path] = Entry{Mode: f[0], ID: f[2]}
	}
	return tree, nil
}

// Walk replays HEAD's first-parent history oldest first, handing fn each
// commit's changes against its diff base. A commit that changed nothing still
// gets a call. A repository with no commits makes none. An error from fn stops
// the walk and is returned.
func (r *Repo) Walk(ctx context.Context, fn func(commit string, changes []Change) error) error {
	head, err := r.Head(ctx)
	if err != nil || head == "" {
		return err
	}
	c := call{args: []string{
		"log", "--first-parent", "--diff-merges=first-parent", "--reverse",
		"--raw", "-z", "--no-renames", "--no-abbrev", "--no-relative", "--no-show-signature",
		"--format=%x01%H", head,
	}}
	return r.stream(ctx, c, func(out io.Reader) error {
		return walkRaw(bufio.NewReader(out), fn)
	})
}

// walkRaw parses the walk's output. Each commit starts with a "\x01<id>" token;
// the raw entries that follow (a metadata token then a path token, the first
// metadata token prefixed by a newline) belong to it until the next header.
func walkRaw(in *bufio.Reader, fn func(commit string, changes []Change) error) error {
	var (
		commit  string
		changes []Change
	)
	flush := func() error {
		if commit == "" {
			return nil
		}
		err := fn(commit, changes)
		commit, changes = "", nil
		return err
	}
	for {
		tok, err := in.ReadString(0)
		if errors.Is(err, io.EOF) {
			if strings.TrimSpace(tok) != "" {
				return fmt.Errorf("walk: truncated output %q", tok)
			}
			return flush()
		}
		if err != nil {
			return err
		}
		tok = strings.TrimSuffix(tok, "\x00")
		if id, ok := strings.CutPrefix(tok, "\x01"); ok {
			if err := flush(); err != nil {
				return err
			}
			commit = id
			continue
		}
		if commit == "" {
			return fmt.Errorf("walk: raw entry %q before any commit", tok)
		}
		path, err := in.ReadString(0)
		if err != nil {
			return fmt.Errorf("walk: entry %q has no path: %w", tok, err)
		}
		c, err := parseRawEntry(tok, strings.TrimSuffix(path, "\x00"))
		if err != nil {
			return err
		}
		changes = append(changes, c)
	}
}
