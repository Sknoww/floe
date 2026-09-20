package editor

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// What floe runs is the whole of this package's behaviour, so it is what the
// tests check: launching a real editor is not something a test can assert on.
func TestCommand(t *testing.T) {
	dir := filepath.FromSlash("/repos/app-public")
	file := filepath.Join(dir, filepath.FromSlash("cmd/server/main.go"))

	cases := []struct {
		name string
		goos string
		cli  string
		path string
		line int
		want []string
	}{{
		name: "VS Code opens the repository at the file",
		goos: "darwin",
		cli:  "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
		path: "cmd/server/main.go",
		line: 41,
		want: []string{dir, "-g", file + ":41"},
	}, {
		name: "no line is the file's top",
		goos: "darwin",
		cli:  "/usr/local/bin/code",
		path: "cmd/server/main.go",
		want: []string{dir, "-g", file},
	}, {
		name: "no file is the repository alone",
		goos: "linux",
		cli:  "/usr/bin/code",
		want: []string{dir},
	}, {
		name: "without VS Code, the default text editor",
		goos: "darwin",
		path: "cmd/server/main.go",
		want: []string{"open", "-t", file},
	}, {
		// A directory has no default text editor, so -t would refuse it.
		name: "without VS Code and without a file, the repository",
		goos: "darwin",
		want: []string{"open", dir},
	}, {
		name: "linux opens through xdg-open",
		goos: "linux",
		path: "cmd/server/main.go",
		want: []string{"xdg-open", file},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd, err := Command(context.Background(), c.goos, c.cli, dir, c.path, c.line)
			if err != nil {
				t.Fatalf("Command: %v", err)
			}
			want := c.want
			if c.cli != "" {
				want = append([]string{c.cli}, want...)
			}
			if !slices.Equal(cmd.Args, want) {
				t.Errorf("ran %q, want %q", cmd.Args, want)
			}
		})
	}
}

// An OS floe has no editor for is told about, rather than silently doing
// nothing when the button is pressed.
func TestCommandUnknownOS(t *testing.T) {
	_, err := Command(context.Background(), "plan9", "", "/repos/app-public", "main.go", 0)
	if err == nil {
		t.Fatal("Command on plan9: want an error")
	}
}
