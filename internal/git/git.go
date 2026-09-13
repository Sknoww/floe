// Package git is a thin wrapper over the git binary. Every call shells out and
// parses machine-readable output; there is no git library.
//
// Reads are the default. The writes are few and named: importing preimage blobs
// into the target's object store, applying a patch to the target's index and
// working tree, resetting it on abort, and the SQUASH_MSG file. Nothing here
// commits, and nothing here touches a remote.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Repo runs git commands against one repository. Dir is its top-level
// directory, as Open resolves it.
type Repo struct {
	Dir string
}

// Error is a git command that exited non-zero. Most callers only report it, but
// a few classify it: `git apply` exits 1 for both a conflict and a failure, and
// `rev-parse --verify -q` exits 1 for a ref that does not exist.
type Error struct {
	Args   []string
	Code   int // the exit code; -1 when git did not run or was killed
	Stderr string
	err    error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.err)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

func (e *Error) Unwrap() error { return e.err }

// exitCode is the exit code of a failed git command, or -1 when err is not one.
func exitCode(err error) int {
	var ge *Error
	if errors.As(err, &ge) {
		return ge.Code
	}
	return -1
}

// baseEnv applies to every call. LC_ALL=C keeps git's messages and sort order
// the same on every machine. The editor variables keep a subprocess from ever
// waiting on an editor nobody will see: floe's terminal belongs to its server.
var baseEnv = []string{
	"LC_ALL=C",
	"GIT_EDITOR=true",
	"GIT_SEQUENCE_EDITOR=true",
	"GIT_TERMINAL_PROMPT=0",
}

// readEnv applies to calls that only read. GIT_OPTIONAL_LOCKS=0 stops `git
// status` from refreshing the index as a side effect, so floe's refreshes never
// contend with the editor's git for the index lock.
var readEnv = []string{"GIT_OPTIONAL_LOCKS=0"}

// call is one git invocation.
type call struct {
	args  []string
	stdin []byte
	env   []string // added to the environment, after baseEnv
	write bool     // the call writes; optional locks stay on
}

// run runs a git command and returns its stdout — also when git fails, for the
// commands whose non-zero exit still comes with output that is wanted:
// merge-file on a conflict, diff --no-index on files that differ.
func (r *Repo) run(ctx context.Context, c call) ([]byte, error) {
	var stdout bytes.Buffer
	err := r.stream(ctx, c, func(out io.Reader) error {
		_, err := io.Copy(&stdout, out)
		return err
	})
	return stdout.Bytes(), err
}

// stream runs a git command and hands its stdout to read as it is produced, for
// output too large to want in memory at once (the position walk). A read that
// returns early kills the command; its error wins over git's.
func (r *Repo) stream(ctx context.Context, c call, read func(io.Reader) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", c.args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), baseEnv...)
	if !c.write {
		cmd.Env = append(cmd.Env, readEnv...)
	}
	cmd.Env = append(cmd.Env, c.env...)
	if c.stdin != nil {
		cmd.Stdin = bytes.NewReader(c.stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return &Error{Args: c.args, Code: -1, err: err}
	}

	readErr := read(out)
	if readErr != nil {
		cancel()
	} else {
		// Drain whatever read left, so git never blocks on a full pipe.
		_, readErr = io.Copy(io.Discard, out)
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			code = ee.ExitCode()
		}
		return &Error{Args: c.args, Code: code, Stderr: strings.TrimSpace(stderr.String()), err: waitErr}
	}
	return nil
}
