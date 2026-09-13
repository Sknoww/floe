package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// refreshPaths refreshes the stat data of the temporary index's entries for
// the paths a patch touches. A read-tree index has none, so to git every file
// on disk looks changed. Adding a file the target already has is an add/add
// merge, which reads the target's file from disk — and a preview refused it as
// not matching the index where the real apply conflicts. The target is clean,
// so what is on disk is HEAD.
//
// Only the patch's paths are refreshed: a whole-index refresh would hash every
// file in the target on every preview.
func (r *Repo) refreshPaths(ctx context.Context, env []string, patch []byte) error {
	// --whitespace is pinned as applyArgs pins it: apply.whitespace=error would
	// refuse even to list a patch that adds a trailing space.
	out, err := r.run(ctx, call{args: []string{"apply", "--numstat", "-z", "--whitespace=nowarn"}, stdin: patch})
	if err != nil {
		return err
	}
	touched := map[string]bool{}
	for _, rec := range nulRecords(out) {
		// "<added>\t<deleted>\t<path>"; with renames off there is one path.
		fields := strings.SplitN(rec, "\t", 3)
		if len(fields) != 3 {
			return fmt.Errorf("apply --numstat: malformed entry %q", rec)
		}
		touched[fields[2]] = true
	}
	out, err = r.run(ctx, call{args: []string{"ls-files", "-z"}, env: env})
	if err != nil {
		return err
	}
	// git add --refresh fails outright on a path the index lacks.
	var present []string
	for _, path := range nulRecords(out) {
		if touched[path] {
			present = append(present, path)
		}
	}
	if len(present) == 0 {
		return nil
	}
	_, err = r.run(ctx, call{
		args:  []string{"add", "--refresh", "--pathspec-from-file=-", "--pathspec-file-nul"},
		stdin: []byte(strings.Join(present, "\x00")),
		env:   append([]string{"GIT_LITERAL_PATHSPECS=1"}, env...),
		write: true,
	})
	return err
}

// landed reads what a preview's result brings into each text file: the lines
// it adds to the target's own version. A clean file's result is its blob in
// the temporary index. A conflict is rebuilt from its stages as the real apply
// writes it, in diff3 style so the base lines count too. Deletions and
// submodules bring in no lines, and binary files are left out: git merges
// none, and the transfer names them.
//
// dir is the preview's temporary directory; the versions compared are written
// there, and nothing else is.
func (r *Repo) landed(ctx context.Context, env []string, dir string, files []Change) (map[string][]string, error) {
	stages, err := r.stages(ctx, env)
	if err != nil {
		return nil, err
	}
	landed := map[string][]string{}
	for _, f := range files {
		var ours, result []byte
		switch {
		case f.Status == 'U':
			var blobs [3][]byte // stages 1, 2 and 3: base, ours, theirs
			for i, id := range stages[f.Path] {
				if id == "" {
					continue // an add/add conflict has no base
				}
				if blobs[i], err = r.blob(ctx, id); err != nil {
					return nil, err
				}
			}
			if isBinary(blobs[0]) || isBinary(blobs[1]) || isBinary(blobs[2]) {
				continue
			}
			ours = blobs[1]
			if result, err = r.mergeFile(ctx, dir, blobs); err != nil {
				return nil, err
			}
		case f.Status == 'D' || f.NewMode == gitlinkMode:
			continue
		default:
			if !isZeroID(f.OldID) && f.OldMode != gitlinkMode {
				if ours, err = r.blob(ctx, f.OldID); err != nil {
					return nil, err
				}
			}
			if result, err = r.blob(ctx, f.NewID); err != nil {
				return nil, err
			}
			if isBinary(ours) || isBinary(result) {
				continue
			}
		}
		lines, err := r.addedLines(ctx, dir, ours, result)
		if err != nil {
			return nil, err
		}
		if len(lines) > 0 {
			landed[f.Path] = lines
		}
	}
	return landed, nil
}

// stages reads the index's unmerged entries by path: the blob ids of stages 1
// (base), 2 (ours) and 3 (theirs), "" for a stage the path lacks.
func (r *Repo) stages(ctx context.Context, env []string) (map[string][3]string, error) {
	out, err := r.run(ctx, call{args: []string{"ls-files", "-u", "-z"}, env: env})
	if err != nil {
		return nil, err
	}
	stages := map[string][3]string{}
	for _, rec := range nulRecords(out) {
		// "<mode> <id> <stage>\t<path>"
		meta, path, ok := strings.Cut(rec, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || len(fields[2]) != 1 || fields[2][0] < '1' || fields[2][0] > '3' {
			return nil, fmt.Errorf("ls-files: malformed entry %q", rec)
		}
		s := stages[path]
		s[fields[2][0]-'1'] = fields[1]
		stages[path] = s
	}
	return stages, nil
}

// blob reads a blob whole.
func (r *Repo) blob(ctx context.Context, id string) ([]byte, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	return r.run(ctx, call{args: []string{"cat-file", "blob", id}})
}

// isBinary is git's own test for a binary file: a NUL in the first 8000 bytes.
func isBinary(b []byte) bool {
	return bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0
}

// mergeFile rebuilds a conflicted file from its base, ours and theirs versions,
// with the markers `git apply --3way` writes, in diff3 style. It merges as git's
// stock text merge does: a custom merge driver is not run.
func (r *Repo) mergeFile(ctx context.Context, dir string, blobs [3][]byte) ([]byte, error) {
	var paths [3]string
	for i, name := range []string{"base", "ours", "theirs"} {
		paths[i] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[i], blobs[i], 0o600); err != nil {
			return nil, err
		}
	}
	out, err := r.run(ctx, call{args: []string{
		"merge-file", "-p", "--diff3", "-L", "ours", "-L", "base", "-L", "theirs",
		paths[1], paths[0], paths[2],
	}})
	// merge-file exits with the number of conflicts, capped at 127; an error
	// is negative, which reaches us as 255.
	if code := exitCode(err); err != nil && (code < 1 || code > 127) {
		return nil, err
	}
	return out, nil
}

// addedLines returns the lines to adds to from, without their "+".
func (r *Repo) addedLines(ctx context.Context, dir string, from, to []byte) ([]string, error) {
	a, b := filepath.Join(dir, "from"), filepath.Join(dir, "to")
	if err := os.WriteFile(a, from, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(b, to, 0o600); err != nil {
		return nil, err
	}
	out, err := r.run(ctx, call{args: []string{
		"diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv",
		"--diff-algorithm=myers", "--unified=0", "--", a, b,
	}})
	// --no-index exits 1 when the files differ.
	if err != nil && exitCode(err) != 1 {
		return nil, err
	}
	var lines []string
	inHunk := false
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "@@ "):
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			lines = append(lines, line[1:])
		}
	}
	return lines, nil
}
