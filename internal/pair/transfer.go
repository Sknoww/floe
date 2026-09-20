package pair

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Transfer records a transfer floe applied that may still wait in the target,
// staged or stopped on a conflict. It is how floe knows, after a restart too,
// that the target's changes are its own to discard: without it, changes in the
// target are the user's, and floe never offers to discard them.
//
// It is written before the apply, and counts only while the target is as the
// transfer left it — HEAD where it was, tracked changes, and the carried message
// waiting. Deciding that takes the target's state, so it is the caller's.
// Nothing about a transfer is written into either repository.
type Transfer struct {
	Commit     string    `json:"commit"`
	TargetHead string    `json:"targetHead"` // "" for a target with no commits
	Skip       []string  `json:"skip"`       // paths unticked for the transfer
	Applied    time.Time `json:"applied"`
}

// TransferPath is where a pair's transfer record lives under root: in
// transfers/, beside pairs/, under the pair's file name.
func TransferPath(root, source, target string) string {
	return filepath.Join(root, "transfers", FileName(source, target))
}

// SaveTransfer writes the pair's transfer record atomically.
func SaveTransfer(root, source, target string, t *Transfer) error {
	if t.Commit == "" {
		return errors.New("a transfer record needs its commit")
	}
	out := *t
	if out.Skip == nil {
		out.Skip = []string{}
	}
	out.Applied = out.Applied.UTC().Truncate(time.Second)
	return writeAtomic(TransferPath(root, source, target), out)
}

// LoadTransfer reads the pair's transfer record: nil when there is none.
func LoadTransfer(root, source, target string) (*Transfer, error) {
	path := TransferPath(root, source, target)
	var t Transfer
	err := readStrict(path, &t)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t.Commit == "" {
		return nil, fmt.Errorf("%s: %q is missing", path, "commit")
	}
	return &t, nil
}

// ClearTransfer removes the pair's transfer record, if there is one.
func ClearTransfer(root, source, target string) error {
	if err := os.Remove(TransferPath(root, source, target)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
