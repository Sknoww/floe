package transfer

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Sknoww/floe/internal/git"
	"github.com/Sknoww/floe/internal/gittest"
)

func TestMain(m *testing.M) { gittest.Main(m) }

func openPair(t *testing.T, src, tgt string) *Pair {
	t.Helper()
	p, err := Open(t.Context(), src, tgt)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func summarize(changes []git.Change) string {
	var parts []string
	for _, c := range changes {
		parts = append(parts, string(c.Status)+" "+c.Path)
	}
	return strings.Join(parts, ", ")
}

// basePair makes a source with a base commit and a change on top, and an
// unrelated target holding the base content, so the change applies cleanly.
func basePair(t *testing.T) (src, tgt, commit string) {
	t.Helper()
	src = gittest.Init(t)
	gittest.Write(t, src, "f.txt", "l1\nl2\nl3\n")
	gittest.Commit(t, src, "Base")
	gittest.Write(t, src, "f.txt", "l1\nSRC\nl3\n")
	commit = gittest.Commit(t, src, "Change f")
	tgt = gittest.Init(t)
	gittest.Write(t, tgt, "f.txt", "l1\nl2\nl3\n")
	gittest.Commit(t, tgt, "Target")
	return src, tgt, commit
}

func TestOpenRefusesPairsThatCannotTransfer(t *testing.T) {
	ctx := t.Context()
	sha1 := gittest.Init(t)
	sha256 := gittest.Init(t, "--object-format=sha256")
	for _, tc := range []struct{ name, src, tgt, want string }{
		{"one repository twice", sha1, sha1, "same repository"},
		{"object formats differ", sha1, sha256, "object format"},
		{"target is not a repository", sha1, t.TempDir(), "target:"},
	} {
		if _, err := Open(ctx, tc.src, tc.tgt); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Open = %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
}

func TestTransferStagesAndCarriesTheMessage(t *testing.T) {
	ctx := t.Context()
	src := gittest.Init(t)
	gittest.Write(t, src, "f.txt", "l1\nl2\nl3\n")
	gittest.Write(t, src, "gone.txt", "bye\n")
	gittest.Commit(t, src, "Base")
	gittest.Write(t, src, "f.txt", "l1\nSRC\nl3\n")
	gittest.Remove(t, src, "gone.txt")
	gittest.Write(t, src, "new/n.txt", "new\n")
	commit := gittest.Commit(t, src, "Change f\n\nWith a body.")

	tgt := gittest.Init(t)
	gittest.Write(t, tgt, "f.txt", "l1\nl2\nl3\n")
	gittest.Write(t, tgt, "gone.txt", "bye\n")
	gittest.Write(t, tgt, "other.txt", "target only\n")
	head := gittest.Commit(t, tgt, "Target")

	p := openPair(t, src, tgt)
	pv, err := p.Preview(ctx, Request{Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	if pv.Message != "Change f\n\nWith a body.\n" || pv.TargetHead != head {
		t.Errorf("preview: message %q, head %s", pv.Message, pv.TargetHead)
	}
	if got := summarize(pv.Result.Files); got != "M f.txt, D gone.txt, A new/n.txt" || len(pv.Result.Conflicts) != 0 {
		t.Errorf("preview result: %q, conflicts %q", got, pv.Result.Conflicts)
	}

	if _, err := p.Apply(ctx, pv); err != nil {
		t.Fatal(err)
	}
	if staged := gittest.Git(t, tgt, "diff", "--cached", "--name-status"); staged != "M\tf.txt\nD\tgone.txt\nA\tnew/n.txt" {
		t.Errorf("staged:\n%s", staged)
	}
	if now := gittest.Git(t, tgt, "rev-parse", "HEAD"); now != head {
		t.Error("the transfer committed")
	}
	if got := gittest.Read(t, tgt, ".git/SQUASH_MSG"); got != pv.Message {
		t.Errorf("SQUASH_MSG = %q, want %q", got, pv.Message)
	}

	// The user's own commit picks the message up, and git removes the file.
	gittest.Git(t, tgt, "commit", "--quiet", "--no-edit")
	if msg := gittest.Git(t, tgt, "log", "-1", "--format=%B"); msg != "Change f\n\nWith a body." {
		t.Errorf("committed message = %q", msg)
	}
	if gittest.Exists(t, tgt, ".git/SQUASH_MSG") {
		t.Error("SQUASH_MSG survived the commit")
	}
}

func TestConflictStopsWithMarkersAndAborts(t *testing.T) {
	ctx := t.Context()
	src, tgt, commit := basePair(t)
	gittest.Write(t, tgt, "f.txt", "l1\nTGT\nl3\n")
	gittest.Commit(t, tgt, "Target edits the same line")

	p := openPair(t, src, tgt)
	pv, err := p.Preview(ctx, Request{Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pv.Result.Conflicts, []string{"f.txt"}) {
		t.Errorf("preview conflicts = %q", pv.Result.Conflicts)
	}
	if status := gittest.Git(t, tgt, "status", "--porcelain"); status != "" {
		t.Errorf("the preview touched the target: %q", status)
	}

	res, err := p.Apply(ctx, pv)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Conflicts, []string{"f.txt"}) || !strings.Contains(gittest.Read(t, tgt, "f.txt"), "<<<<<<<") {
		t.Errorf("apply: conflicts %q, f.txt:\n%s", res.Conflicts, gittest.Read(t, tgt, "f.txt"))
	}
	st, err := p.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(st.Conflicts, []string{"f.txt"}) || !st.Message {
		t.Errorf("state after conflict: %+v", st)
	}

	if err := p.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	if status := gittest.Git(t, tgt, "status", "--porcelain"); status != "" {
		t.Errorf("after abort: %q", status)
	}
	if got := gittest.Read(t, tgt, "f.txt"); got != "l1\nTGT\nl3\n" {
		t.Errorf("after abort, f.txt = %q", got)
	}
	if gittest.Exists(t, tgt, ".git/SQUASH_MSG") {
		t.Error("abort left SQUASH_MSG")
	}
}

func TestDirtyTargetIsRefused(t *testing.T) {
	ctx := t.Context()
	src, tgt, commit := basePair(t)
	p := openPair(t, src, tgt)

	gittest.Write(t, tgt, "f.txt", "uncommitted\n")
	var de *DirtyError
	if _, err := p.Preview(ctx, Request{Commit: commit}); !errors.As(err, &de) || !slices.Equal(de.Paths, []string{"f.txt"}) {
		t.Fatalf("Preview into a dirty target = %v", err)
	}

	gittest.Git(t, tgt, "checkout", "--", "f.txt")
	gittest.Write(t, tgt, "scratch.txt", "untracked\n")
	if _, err := p.Preview(ctx, Request{Commit: commit}); err != nil {
		t.Errorf("an untracked file refused the transfer: %v", err)
	}
}

func TestStaleSquashMsgIsRemoved(t *testing.T) {
	ctx := t.Context()
	src, tgt, _ := basePair(t)
	gittest.Write(t, tgt, ".git/SQUASH_MSG", "left behind\n")
	st, err := openPair(t, src, tgt).State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Message || gittest.Exists(t, tgt, ".git/SQUASH_MSG") {
		t.Error("a SQUASH_MSG beside a clean tree survived")
	}
}

func TestExcludedAndUntickedFilesNeverCross(t *testing.T) {
	ctx := t.Context()
	src := gittest.Init(t)
	gittest.Write(t, src, "README.md", "internal readme\n")
	gittest.Write(t, src, ".github/ci.yml", "internal ci\n")
	gittest.Write(t, src, "app.go", "package app\n")
	gittest.Write(t, src, "lib.go", "package lib\n")
	gittest.Commit(t, src, "Base")
	gittest.Write(t, src, "README.md", "internal readme v2\n")
	gittest.Write(t, src, ".github/ci.yml", "internal ci v2\n")
	gittest.Write(t, src, "app.go", "package app\n\nfunc A() {}\n")
	gittest.Write(t, src, "lib.go", "package lib\n\nfunc L() {}\n")
	commit := gittest.Commit(t, src, "Change everything")

	tgt := gittest.Init(t)
	gittest.Write(t, tgt, "README.md", "public readme\n")
	gittest.Write(t, tgt, "app.go", "package app\n")
	gittest.Write(t, tgt, "lib.go", "package lib\n")
	gittest.Commit(t, tgt, "Target")

	excluded := Excluded(func(path string) bool {
		return path == "README.md" || strings.HasPrefix(path, ".github/")
	})
	p := openPair(t, src, tgt)
	pv, err := p.Preview(ctx, Request{Commit: commit, Excluded: excluded, Skip: []string{"lib.go"}})
	if err != nil {
		t.Fatal(err)
	}
	var marks []string
	for _, f := range pv.Files {
		marks = append(marks, f.Path+map[[2]bool]string{{false, false}: "", {true, false}: " excluded", {false, true}: " skipped"}[[2]bool{f.Excluded, f.Skipped}])
	}
	if want := []string{".github/ci.yml excluded", "README.md excluded", "app.go", "lib.go skipped"}; !slices.Equal(marks, want) {
		t.Errorf("files = %q, want %q", marks, want)
	}
	if n := strings.Count(string(pv.patch), "diff --git "); n != 1 || !strings.Contains(string(pv.patch), "diff --git a/app.go b/app.go") {
		t.Errorf("patch carries more than app.go:\n%s", pv.patch)
	}

	if _, err := p.Apply(ctx, pv); err != nil {
		t.Fatal(err)
	}
	if staged := gittest.Git(t, tgt, "diff", "--cached", "--name-status"); staged != "M\tapp.go" {
		t.Errorf("staged:\n%s", staged)
	}
	if gittest.Read(t, tgt, "README.md") != "public readme\n" || gittest.Exists(t, tgt, ".github") {
		t.Error("an excluded file crossed")
	}

	if err := p.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Preview(ctx, Request{Commit: commit, Excluded: excluded, Skip: []string{"app.go", "lib.go"}}); !errors.Is(err, ErrNothingToTransfer) {
		t.Errorf("everything left out: Preview = %v", err)
	}
}

func TestMoveOutOfAnExcludedDirectoryCarriesOnlyTheAdd(t *testing.T) {
	ctx := t.Context()
	src := gittest.Init(t)
	gittest.Write(t, src, "internal/notes.txt", "shareable now\n")
	gittest.Commit(t, src, "Base")
	gittest.Remove(t, src, "internal/notes.txt")
	gittest.Write(t, src, "public/notes.txt", "shareable now\n")
	commit := gittest.Commit(t, src, "Publish the notes")

	tgt := gittest.Init(t)
	gittest.Write(t, tgt, "x.txt", "x\n")
	gittest.Commit(t, tgt, "Target")

	excluded := Excluded(func(path string) bool { return strings.HasPrefix(path, "internal/") })
	pv, err := openPair(t, src, tgt).Preview(ctx, Request{Commit: commit, Excluded: excluded})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pv.patch), "internal/") {
		t.Errorf("the patch names the excluded path:\n%s", pv.patch)
	}
	if got := summarize(pv.Result.Files); got != "A public/notes.txt" {
		t.Errorf("result = %q", got)
	}
}

func TestContentGuard(t *testing.T) {
	ctx := t.Context()
	guard := []*regexp.Regexp{regexp.MustCompile(`(?i)acme corp`)}

	src := gittest.Init(t)
	gittest.Write(t, src, "w.txt", "w1\n")
	gittest.Write(t, src, "y.txt", "old ACME Corp line\nkeep\n")
	gittest.Commit(t, src, "Base")
	gittest.Write(t, src, "w.txt", "w2\n")
	gittest.Write(t, src, "x.txt", "calls the Acme Corp API\n")
	added := gittest.Commit(t, src, "Add x")
	gittest.Write(t, src, "y.txt", "keep\n")
	removed := gittest.Commit(t, src, "Scrub y")
	gittest.Write(t, src, "q.txt", "q\n")
	message := gittest.Commit(t, src, "Roll out for ACME CORP")
	gittest.Write(t, src, "bin.dat", "\x00\x01\x02\xff")
	binary := gittest.Commit(t, src, "Add a binary")

	tgt := gittest.Init(t)
	gittest.Write(t, tgt, "w.txt", "target's w\n")
	gittest.Write(t, tgt, "y.txt", "old ACME Corp line\nkeep\n")
	gittest.Commit(t, tgt, "Target")
	p := openPair(t, src, tgt)

	// An added line refuses, before the preimage import writes anything.
	var ge *GuardError
	if _, err := p.Preview(ctx, Request{Commit: added, Guard: guard}); !errors.As(err, &ge) {
		t.Fatalf("added line: Preview = %v, want *GuardError", err)
	}
	if want := []GuardMatch{{Pattern: "(?i)acme corp", Path: "x.txt", Line: "calls the Acme Corp API"}}; !slices.Equal(ge.Matches, want) {
		t.Errorf("matches = %+v, want %+v", ge.Matches, want)
	}
	wBlob := gittest.Git(t, src, "rev-parse", added+"^:w.txt")
	if missing, _ := p.Target.MissingObjects(ctx, []string{wBlob}); len(missing) != 1 {
		t.Error("a guard refusal imported the preimage anyway")
	}

	// A removed line is not scanned: scrubbing a match is the point.
	if _, err := p.Preview(ctx, Request{Commit: removed, Guard: guard}); err != nil {
		t.Errorf("removed line: Preview = %v", err)
	}

	// The message is scanned.
	if _, err := p.Preview(ctx, Request{Commit: message, Guard: guard}); !errors.As(err, &ge) || ge.Matches[0].Path != "" {
		t.Errorf("message: Preview = %v, want a *GuardError on the message", err)
	}

	// A binary file cannot be scanned, and is named.
	pv, err := p.Preview(ctx, Request{Commit: binary, Guard: guard})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pv.Binary, []string{"bin.dat"}) {
		t.Errorf("Binary = %q", pv.Binary)
	}
}

func TestContentGuardScansWhatAMergeBringsIn(t *testing.T) {
	ctx := t.Context()
	guard := []*regexp.Regexp{regexp.MustCompile(`(?i)acme corp`)}

	// The source changes line 8. The patch's context is lines 5 to 11, so the
	// guard-matching line 3 is not in it — but the target rewrote lines 3 to
	// 9, and the conflict's source side carries line 3 across.
	src := gittest.Init(t)
	gittest.Write(t, src, "f.txt", "l1\nl2\nACME Corp internal\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nl12\n")
	gittest.Write(t, src, "own.txt", "Acme Corp mirror\nx\n")
	gittest.Commit(t, src, "Base")
	gittest.Write(t, src, "f.txt", "l1\nl2\nACME Corp internal\nl4\nl5\nl6\nl7\nSRC\nl9\nl10\nl11\nl12\n")
	conflicting := gittest.Commit(t, src, "Change line 8")
	gittest.Write(t, src, "own.txt", "Acme Corp mirror\ny\n")
	beside := gittest.Commit(t, src, "Change beside a line the target has too")

	tgt := gittest.Init(t)
	gittest.Write(t, tgt, "f.txt", "l1\nl2\nscrubbed\nT4\nT5\nT6\nT7\nT8\nT9\nl10\nl11\nl12\n")
	gittest.Write(t, tgt, "own.txt", "Acme Corp mirror\nx\n")
	gittest.Commit(t, tgt, "Target")
	p := openPair(t, src, tgt)

	var ge *GuardError
	if _, err := p.Preview(ctx, Request{Commit: conflicting, Guard: guard}); !errors.As(err, &ge) {
		t.Fatalf("Preview = %v, want *GuardError", err)
	}
	want := GuardMatch{Pattern: "(?i)acme corp", Path: "f.txt", Line: "ACME Corp internal", Merged: true}
	for _, m := range ge.Matches {
		if m != want {
			t.Errorf("match %+v, want %+v", m, want)
		}
	}
	if status := gittest.Git(t, tgt, "status", "--porcelain"); status != "" {
		t.Errorf("a refusal touched the target: %q", status)
	}

	// A match the target already has — here, as the patch's context — is not
	// something the transfer brings in.
	if _, err := p.Preview(ctx, Request{Commit: beside, Guard: guard}); err != nil {
		t.Errorf("Preview beside the target's own match = %v", err)
	}
}

func TestApplyChecksTheTargetAgain(t *testing.T) {
	ctx := t.Context()
	src, tgt, commit := basePair(t)
	p := openPair(t, src, tgt)

	pv, err := p.Preview(ctx, Request{Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	gittest.Write(t, tgt, "later.txt", "moved on\n")
	gittest.Commit(t, tgt, "Target moves")
	if _, err := p.Apply(ctx, pv); !errors.Is(err, ErrStale) {
		t.Errorf("Apply after HEAD moved = %v, want ErrStale", err)
	}

	pv, err = p.Preview(ctx, Request{Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	gittest.Write(t, tgt, "later.txt", "uncommitted\n")
	var de *DirtyError
	if _, err := p.Apply(ctx, pv); !errors.As(err, &de) {
		t.Errorf("Apply into a target dirtied since = %v, want *DirtyError", err)
	}
}

func TestEmptyTarget(t *testing.T) {
	ctx := t.Context()
	src := gittest.Init(t)
	gittest.Write(t, src, "a.txt", "a\n")
	gittest.Write(t, src, "d/sub/b.txt", "b\n")
	root := gittest.Commit(t, src, "Root")

	tgt := gittest.Init(t)
	gittest.Write(t, tgt, "keep.txt", "untracked\n")
	p := openPair(t, src, tgt)

	pv, err := p.Preview(ctx, Request{Commit: root})
	if err != nil {
		t.Fatal(err)
	}
	if pv.TargetHead != "" || summarize(pv.Result.Files) != "A a.txt, A d/sub/b.txt" {
		t.Errorf("preview: head %q, result %q", pv.TargetHead, summarize(pv.Result.Files))
	}
	if _, err := p.Apply(ctx, pv); err != nil {
		t.Fatal(err)
	}
	if err := p.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	if gittest.Exists(t, tgt, "a.txt") || gittest.Exists(t, tgt, "d") || !gittest.Exists(t, tgt, "keep.txt") {
		t.Error("abort in an empty target left the transfer behind or took the untracked file")
	}

	// Transferred again and committed, it becomes the target's first commit.
	if pv, err = p.Preview(ctx, Request{Commit: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Apply(ctx, pv); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, tgt, "commit", "--quiet", "--no-edit")
	if msg := gittest.Git(t, tgt, "log", "-1", "--format=%B"); msg != "Root" {
		t.Errorf("first commit message = %q", msg)
	}
}

func TestMergeCommitCarriesItsBranch(t *testing.T) {
	ctx := t.Context()
	src := gittest.Init(t)
	gittest.Write(t, src, "a.txt", "a\n")
	gittest.Commit(t, src, "Base")
	gittest.Git(t, src, "checkout", "--quiet", "-b", "feature")
	gittest.Write(t, src, "b.txt", "b\n")
	gittest.Commit(t, src, "Add b")
	gittest.Write(t, src, "c.txt", "c\n")
	gittest.Commit(t, src, "Add c")
	gittest.Git(t, src, "checkout", "--quiet", "main")
	gittest.Git(t, src, "merge", "--quiet", "--no-ff", "-m", "Merge feature", "feature")
	merge := gittest.Git(t, src, "rev-parse", "HEAD")

	tgt := gittest.Init(t)
	gittest.Write(t, tgt, "a.txt", "a\n")
	gittest.Commit(t, tgt, "Target")

	pv, err := openPair(t, src, tgt).Preview(ctx, Request{Commit: merge})
	if err != nil {
		t.Fatal(err)
	}
	if got := summarize(pv.Result.Files); got != "A b.txt, A c.txt" || pv.Message != "Merge feature\n" {
		t.Errorf("merge preview: %q, message %q", got, pv.Message)
	}
}

func TestPosition(t *testing.T) {
	ctx := t.Context()
	src := gittest.Init(t)
	step := func(msg string, files map[string]string) string {
		for path, content := range files {
			gittest.Write(t, src, path, content)
		}
		gittest.Write(t, src, "README.md", "internal, as of "+msg+"\n")
		return gittest.Commit(t, src, msg)
	}
	step("c1", map[string]string{"a": "1\n"})
	c2 := step("c2", map[string]string{"b": "1\n"})
	c3 := step("c3", map[string]string{"a": "2\n"})
	c4 := step("c4", map[string]string{"b": "2\n"})
	target := func(files map[string]string) string {
		dir := gittest.Init(t)
		for path, content := range files {
			gittest.Write(t, dir, path, content)
		}
		gittest.Write(t, dir, "README.md", "public\n")
		gittest.Commit(t, dir, "Target")
		return dir
	}
	readme := Excluded(func(path string) bool { return path == "README.md" })

	position := func(tgt string, excluded Excluded) Position {
		t.Helper()
		pos, err := openPair(t, src, tgt).Position(ctx, excluded)
		if err != nil {
			t.Fatal(err)
		}
		return *pos
	}
	check := func(name string, got, want Position) {
		t.Helper()
		if got.Match != want.Match || got.Nearest != want.Nearest || !slices.Equal(got.Divergent, want.Divergent) {
			t.Errorf("%s: Position = %+v, want %+v", name, got, want)
		}
	}

	atC2 := target(map[string]string{"a": "1\n", "b": "1\n"})
	check("match, README excluded", position(atC2, readme), Position{Match: c2})
	check("no exclusions", position(atC2, nil), Position{Nearest: c2, Divergent: []string{"README.md"}})
	check("nearest", position(target(map[string]string{"a": "2\n", "b": "1\n", "extra": "x\n"}), readme),
		Position{Nearest: c3, Divergent: []string{"extra"}})
	check("match at the tip", position(target(map[string]string{"a": "2\n", "b": "2\n"}), readme), Position{Match: c4})

	c5 := step("c5 reverts to c2", map[string]string{"a": "1\n", "b": "1\n"})
	check("a tie goes to the newest", position(atC2, readme), Position{Match: c5})

	empty, err := openPair(t, gittest.Init(t), atC2).Position(ctx, readme)
	if err != nil || empty.Match != "" || empty.Nearest != "" {
		t.Errorf("empty source: Position = %+v, %v", empty, err)
	}
}
