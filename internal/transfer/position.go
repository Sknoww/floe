package transfer

import (
	"context"
	"sort"

	"github.com/Sknoww/floe/internal/git"
)

// Position is where the target stands against the source's first-parent
// history. It is computed from blob ids, exclusions removed on both sides, and
// never stored in either repository.
type Position struct {
	// Match is the newest source commit whose tree equals the target's.
	Match string
	// Nearest, when nothing matches, is the newest source commit with the
	// fewest paths differing from the target, and Divergent lists those paths.
	Nearest   string
	Divergent []string
}

// Position compares the target's HEAD with every commit on the source's
// first-parent history.
//
// The source side is replayed oldest first, keeping a running path → entry map
// and a running count of paths that differ from the target. Each commit updates
// only the paths it changed, so the cost is the number of changes, not commits ×
// files. Ties go to the newest commit: a history that returns to an earlier
// state, a revert say, is placed at its latest point, which is where the next
// transfer would start from.
func (p *Pair) Position(ctx context.Context, excluded Excluded) (*Position, error) {
	head, err := p.Target.Head(ctx)
	if err != nil {
		return nil, err
	}
	target, err := p.Target.Tree(ctx, head)
	if err != nil {
		return nil, err
	}
	dropExcluded(target, excluded)

	source := map[string]git.Entry{}
	differ := len(target) // against nothing, every target path differs
	best, fewest := "", -1
	err = p.Source.Walk(ctx, func(commit string, changes []git.Change) error {
		for _, c := range changes {
			if excluded.has(c.Path) {
				continue
			}
			before := sameEntry(source, target, c.Path)
			if c.Status == 'D' {
				delete(source, c.Path)
			} else {
				source[c.Path] = git.Entry{Mode: c.NewMode, ID: c.NewID}
			}
			switch after := sameEntry(source, target, c.Path); {
			case before && !after:
				differ++
			case !before && after:
				differ--
			}
		}
		if fewest < 0 || differ <= fewest {
			best, fewest = commit, differ
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	switch {
	case best == "":
		return &Position{}, nil
	case fewest == 0:
		return &Position{Match: best}, nil
	}
	tree, err := p.Source.Tree(ctx, best)
	if err != nil {
		return nil, err
	}
	dropExcluded(tree, excluded)
	return &Position{Nearest: best, Divergent: differingPaths(tree, target)}, nil
}

func dropExcluded(tree map[string]git.Entry, excluded Excluded) {
	for path := range tree {
		if excluded.has(path) {
			delete(tree, path)
		}
	}
}

// sameEntry reports whether path is the same file on both sides: present in
// both with the same mode and id, or absent from both.
func sameEntry(a, b map[string]git.Entry, path string) bool {
	ea, inA := a[path]
	eb, inB := b[path]
	return inA == inB && ea == eb
}

// differingPaths lists, sorted, every path that is not the same file in a and b.
func differingPaths(a, b map[string]git.Entry) []string {
	var paths []string
	for path := range a {
		if !sameEntry(a, b, path) {
			paths = append(paths, path)
		}
	}
	for path := range b {
		if _, inA := a[path]; !inA {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}
