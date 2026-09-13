// Package transfer carries one source commit into the target. It owns the order
// of the steps: every check that can refuse a transfer runs before anything is
// written, so a refusal has nothing to undo.
package transfer

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/Sknoww/floe/internal/git"
)

// Pair is a source repository and the target its commits are carried into.
type Pair struct {
	Source *git.Repo
	Target *git.Repo
}

// Open resolves both repositories and refuses a pair floe cannot transfer
// between: a git older than git.MinVersion, one repository named twice, or two
// repositories whose object formats differ. Blob ids are the whole mechanism,
// and a SHA-1 repository and a SHA-256 one share none.
func Open(ctx context.Context, source, target string) (*Pair, error) {
	if err := git.CheckVersion(ctx); err != nil {
		return nil, err
	}
	src, err := git.Open(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	tgt, err := git.Open(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	if src.Dir == tgt.Dir {
		return nil, fmt.Errorf("the source and target are the same repository: %s", src.Dir)
	}
	srcFormat, err := src.ObjectFormat(ctx)
	if err != nil {
		return nil, err
	}
	tgtFormat, err := tgt.ObjectFormat(ctx)
	if err != nil {
		return nil, err
	}
	if srcFormat != tgtFormat {
		return nil, fmt.Errorf("the source uses %s object ids and the target %s; both must use the same object format", srcFormat, tgtFormat)
	}
	return &Pair{Source: src, Target: tgt}, nil
}

// Excluded reports whether a repo-relative path must never be transferred. The
// caller matches the pair's patterns; this package only asks. A nil Excluded
// excludes nothing.
type Excluded func(path string) bool

func (e Excluded) has(path string) bool { return e != nil && e(path) }

// File is one path a source commit changed.
type File struct {
	git.Change
	Excluded bool // matches the pair's exclusions: never transferred
	Skipped  bool // unticked for this transfer only
}

// Files lists the paths a source commit changed. Excluded paths are listed and
// marked rather than dropped, so what will not cross stays visible.
func (p *Pair) Files(ctx context.Context, commit string, excluded Excluded) ([]File, error) {
	changes, err := p.Source.Changes(ctx, commit)
	if err != nil {
		return nil, err
	}
	files := make([]File, len(changes))
	for i, c := range changes {
		files[i] = File{Change: c, Excluded: excluded.has(c.Path)}
	}
	return files, nil
}

// Request is one transfer asked for.
type Request struct {
	Commit   string
	Excluded Excluded
	Skip     []string         // paths unticked for this transfer
	Guard    []*regexp.Regexp // the pair's content guard
}

// Preview is what a transfer will do, computed without touching the target's
// index or working tree. It is what the user confirms, and Apply carries out
// exactly it.
type Preview struct {
	Commit     string
	TargetHead string // "" for a target with no commits
	Message    string // the source commit's full message
	Files      []File
	// Binary lists the crossing files whose patch is binary. Binary patches are
	// base85-encoded and the content guard cannot scan them, so they are named:
	// a visible gap rather than a silent one.
	Binary []string
	Result *git.ApplyResult
	patch  []byte
}

// DirtyError refuses a transfer into a target with changes to tracked files, so
// a transfer never mixes with other uncommitted work — another transfer
// included — in one commit.
type DirtyError struct {
	Paths []string
}

func (e *DirtyError) Error() string {
	return fmt.Sprintf("the target has uncommitted changes to %d tracked file(s); commit or discard them first", len(e.Paths))
}

var (
	// ErrEmptyCommit refuses a commit that changed no files.
	ErrEmptyCommit = errors.New("the commit changes no files")
	// ErrNothingToTransfer refuses a transfer whose every file is excluded or
	// unticked.
	ErrNothingToTransfer = errors.New("every file the commit changed is excluded or unticked")
	// ErrStale refuses to apply a preview computed against a target HEAD that
	// has since moved. The preview is run again rather than a stale one applied.
	ErrStale = errors.New("the target's HEAD moved since the preview")
)

// Preview runs every check that can refuse the transfer, and only then writes:
//
//  1. The target is clean.
//  2. The content guard passes on the patch's added lines and the message.
//  3. The preimage blobs are imported, and the result is computed on a
//     temporary index.
//  4. The content guard passes on what the result brings into each file.
//
// A patch git would refuse comes back as a *git.ApplyError.
func (p *Pair) Preview(ctx context.Context, req Request) (*Preview, error) {
	head, err := p.cleanTarget(ctx)
	if err != nil {
		return nil, err
	}

	files, err := p.Files(ctx, req.Commit, req.Excluded)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrEmptyCommit
	}
	skip := make(map[string]bool, len(req.Skip))
	for _, path := range req.Skip {
		skip[path] = true
	}
	var left []string
	var crossing []git.Change
	for i := range files {
		files[i].Skipped = skip[files[i].Path]
		if files[i].Excluded || files[i].Skipped {
			left = append(left, files[i].Path)
		} else {
			crossing = append(crossing, files[i].Change)
		}
	}
	if len(crossing) == 0 {
		return nil, ErrNothingToTransfer
	}
	patch, err := p.Source.Patch(ctx, req.Commit, left)
	if err != nil {
		return nil, err
	}
	message, err := p.Source.Message(ctx, req.Commit)
	if err != nil {
		return nil, err
	}
	sections, err := splitPatch(patch)
	if err != nil {
		return nil, err
	}
	if matches := guardMatches(req.Guard, sections, message); len(matches) > 0 {
		return nil, &GuardError{Matches: matches}
	}
	var binary []string
	for _, s := range sections {
		if s.Binary {
			binary = append(binary, s.Path)
		}
	}

	if _, err := p.Target.ImportBlobs(ctx, p.Source, git.Preimages(crossing)); err != nil {
		return nil, err
	}
	result, err := p.Target.Preview(ctx, head, patch)
	if err != nil {
		return nil, err
	}
	// A 3-way merge can bring in source lines the patch has only as context,
	// or not at all: both sides of a conflict, the diff3 base, whatever a merge
	// driver keeps. So what lands is scanned too. Only unreachable objects have
	// been written, so a refusal still has nothing visible to undo.
	if matches := landedMatches(req.Guard, result.Landed); len(matches) > 0 {
		return nil, &GuardError{Matches: matches}
	}
	return &Preview{
		Commit:     req.Commit,
		TargetHead: head,
		Message:    message,
		Files:      files,
		Binary:     binary,
		Result:     result,
		patch:      patch,
	}, nil
}

// Apply carries out a preview:
//
//  5. The target is checked again: still clean, and HEAD where the preview saw
//     it.
//  6. The patch is applied to the index and working tree with a 3-way merge.
//  7. The source commit's message is written to SQUASH_MSG for the commit the
//     user writes — after a conflict too, since the message outlives resolving
//     it.
//
// A *git.ApplyError has written nothing.
func (p *Pair) Apply(ctx context.Context, pv *Preview) (*git.ApplyResult, error) {
	head, err := p.cleanTarget(ctx)
	if err != nil {
		return nil, err
	}
	if head != pv.TargetHead {
		return nil, ErrStale
	}
	result, err := p.Target.Apply(ctx, pv.patch)
	if err != nil {
		return nil, err
	}
	if err := p.Target.WriteSquashMsg(ctx, pv.Message); err != nil {
		return nil, err
	}
	return result, nil
}

// Abort discards a transfer that is staged or stopped on a conflict: the target
// returns to its HEAD and the carried message is removed. Edits made since,
// conflict resolutions included, are discarded with it, so the caller confirms
// first. It is safe only because a transfer starts from a clean target: there
// was nothing else uncommitted to lose, and untracked files stay.
func (p *Pair) Abort(ctx context.Context) error {
	if err := p.Target.ResetHard(ctx); err != nil {
		return err
	}
	_, err := p.Target.RemoveSquashMsg(ctx)
	return err
}

// TargetState is where the target stands between transfers.
type TargetState struct {
	Head      string   // "" for a target with no commits
	Dirty     []string // tracked paths with changes: a staged transfer, or other work
	Conflicts []string // paths left with conflict markers
	Message   bool     // a carried message waits in SQUASH_MSG
}

// State reads the target's state, removing a stale SQUASH_MSG on the way.
func (p *Pair) State(ctx context.Context) (*TargetState, error) {
	head, err := p.Target.Head(ctx)
	if err != nil {
		return nil, err
	}
	dirty, err := p.Target.Dirty(ctx)
	if err != nil {
		return nil, err
	}
	conflicts, err := p.Target.Unmerged(ctx)
	if err != nil {
		return nil, err
	}
	if len(dirty) == 0 {
		if _, err := p.Target.RemoveSquashMsg(ctx); err != nil {
			return nil, err
		}
	}
	message, err := p.Target.HasSquashMsg(ctx)
	if err != nil {
		return nil, err
	}
	return &TargetState{Head: head, Dirty: dirty, Conflicts: conflicts, Message: message}, nil
}

// cleanTarget refuses a target with changes to tracked files and returns its
// HEAD. A SQUASH_MSG beside a clean tree is stale, whoever wrote it — git
// deletes the file on commit, and a clean tree has nothing for it to describe —
// so it is removed before it can pre-fill an unrelated commit.
func (p *Pair) cleanTarget(ctx context.Context) (string, error) {
	dirty, err := p.Target.Dirty(ctx)
	if err != nil {
		return "", err
	}
	if len(dirty) > 0 {
		return "", &DirtyError{Paths: dirty}
	}
	if _, err := p.Target.RemoveSquashMsg(ctx); err != nil {
		return "", err
	}
	return p.Target.Head(ctx)
}
