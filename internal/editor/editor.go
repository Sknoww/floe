// Package editor opens the target repository in the user's editor, at the file
// floe stopped on.
//
// It launches VS Code, which is what the conflict and staged screens send the
// user to: its commit box picks up the carried message floe wrote to
// SQUASH_MSG, and it has conflict tooling. Where VS Code is not installed the
// file opens in the system's default text editor instead.
//
// $VISUAL and $EDITOR are not consulted: floe is driven from a browser, and a
// terminal editor has no terminal to open in.
package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// bundles are the application bundles VS Code installs into on macOS, in the
// order they are preferred. `code` is a shell script inside the bundle — the
// same one a PATH installation points at — and talks to a running instance, so
// floe runs it directly rather than through `open -b <id> --args`, whose
// arguments an already-running application never sees.
var bundles = []string{
	"/Applications/Visual Studio Code.app",
	"/Applications/Visual Studio Code - Insiders.app",
	"~/Applications/Visual Studio Code.app",
	"~/Applications/Visual Studio Code - Insiders.app",
}

// cliInBundle is where a macOS bundle keeps the command line tool.
const cliInBundle = "Contents/Resources/app/bin/code"

// Find is the VS Code command line tool, or "" where VS Code is not installed.
// On macOS the command is often not on PATH even with VS Code installed, so
// floe looks in the application bundles itself rather than relying on it.
func Find() string {
	for _, name := range []string{"code", "code-insiders"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, _ := os.UserHomeDir()
	for _, bundle := range bundles {
		if rest, ok := strings.CutPrefix(bundle, "~/"); ok {
			if home == "" {
				continue
			}
			bundle = filepath.Join(home, rest)
		}
		cli := filepath.Join(bundle, cliInBundle)
		if fi, err := os.Stat(cli); err == nil && fi.Mode().IsRegular() {
			return cli
		}
	}
	return ""
}

// Command is what floe runs to open dir — and path within it, when there is
// one — in the editor. cli is VS Code's command line tool as Find returns it;
// without it the file opens in the system's default text editor. line numbers
// from 1, and 0 means the file's top.
//
// path is repo-relative and in slash form, and is joined onto dir. Joining
// cleans away "..", so the join is not itself a boundary: the caller passes
// only a path git reported in the repository.
func Command(ctx context.Context, goos, cli, dir, path string, line int) (*exec.Cmd, error) {
	full := dir
	if path != "" {
		full = filepath.Join(dir, filepath.FromSlash(path))
	}
	if cli != "" {
		// The repository is opened as a folder, so the commit box that picks
		// up the carried message is there. -g puts the cursor in the file.
		args := []string{dir}
		if path != "" {
			at := full
			if line > 0 {
				at += ":" + strconv.Itoa(line)
			}
			args = append(args, "-g", at)
		}
		return exec.CommandContext(ctx, cli, args...), nil
	}
	switch goos {
	case "darwin":
		// -t is the default text editor; a directory has none, and opens
		// wherever the system opens a folder.
		if path != "" {
			return exec.CommandContext(ctx, "open", "-t", full), nil
		}
		return exec.CommandContext(ctx, "open", full), nil
	case "linux":
		return exec.CommandContext(ctx, "xdg-open", full), nil
	}
	return nil, fmt.Errorf("floe does not know how to open an editor on %s: install VS Code, or open %s yourself", goos, full)
}

// Open launches the editor and returns once it has started. The command is not
// waited on: an editor outlives the request that opened it.
func Open(ctx context.Context, dir, path string, line int) error {
	// The context is the process's, not the request's: a page that goes away
	// mid-launch must not kill the editor it asked for.
	cmd, err := Command(context.WithoutCancel(ctx), runtime.GOOS, Find(), dir, path, line)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", cmd.Path, err)
	}
	go cmd.Wait() // release the process once it exits
	return nil
}
