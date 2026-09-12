package git

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Sknoww/floe/internal/gittest"
)

func TestMain(m *testing.M) { gittest.Main(m) }

func open(t *testing.T, dir string) *Repo {
	t.Helper()
	r, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// summarize renders changes as "M a.txt, A b.txt" for comparison.
func summarize(changes []Change) string {
	var parts []string
	for _, c := range changes {
		parts = append(parts, string(c.Status)+" "+c.Path)
	}
	return strings.Join(parts, ", ")
}

func TestParseVersion(t *testing.T) {
	for _, tc := range []struct {
		out  string
		want Version
	}{
		{"git version 2.50.1 (Apple Git-155)\n", Version{2, 50, 1}},
		{"git version 2.45.2.windows.1\n", Version{2, 45, 2}},
		{"git version 2.32\n", Version{2, 32, 0}},
	} {
		got, err := parseVersion(tc.out)
		if err != nil || got != tc.want {
			t.Errorf("parseVersion(%q) = %v, %v; want %v", tc.out, got, err, tc.want)
		}
	}
	if _, err := parseVersion("hub version 1.0"); err == nil {
		t.Error("parseVersion accepted output that is not git's")
	}
	if !(Version{2, 31, 9}).Less(MinVersion) || MinVersion.Less(Version{2, 32, 0}) || (Version{3, 0, 0}).Less(MinVersion) {
		t.Error("Version.Less orders releases wrongly")
	}
}

func TestInstalledGitMeetsTheFloor(t *testing.T) {
	if err := CheckVersion(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestOpenResolvesTheTopLevel(t *testing.T) {
	ctx := t.Context()
	dir := gittest.Init(t)
	top, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{dir, sub, link} {
		if got := open(t, in).Dir; got != top {
			t.Errorf("Open(%s).Dir = %s, want %s", in, got, top)
		}
	}

	bare := t.TempDir()
	gittest.Git(t, bare, "init", "--quiet", "--bare")
	if _, err := Open(ctx, bare); err == nil || !strings.Contains(err.Error(), "bare") {
		t.Errorf("Open(bare) = %v, want a bare-repository error", err)
	}
	if _, err := Open(ctx, t.TempDir()); err == nil {
		t.Error("Open accepted a directory that is not a repository")
	}
}

func TestCheckIDRefusesAnythingButAFullID(t *testing.T) {
	for _, id := range []string{"HEAD", "--output=/tmp/x", "abc1234", strings.Repeat("g", 40)} {
		if checkID(id) == nil {
			t.Errorf("checkID(%q) accepted it", id)
		}
	}
	for _, id := range []string{strings.Repeat("a", 40), strings.Repeat("0", 64)} {
		if err := checkID(id); err != nil {
			t.Errorf("checkID(%q) = %v", id, err)
		}
	}
}

func TestEmptyRepositoryHasNoHistory(t *testing.T) {
	ctx := t.Context()
	dir := gittest.Init(t)
	r := open(t, dir)
	if head, err := r.Head(ctx); err != nil || head != "" {
		t.Errorf("Head = %q, %v; want empty", head, err)
	}
	if commits, err := r.Commits(ctx); err != nil || len(commits) != 0 {
		t.Errorf("Commits = %v, %v; want none", commits, err)
	}
	calls := 0
	if err := r.Walk(ctx, func(string, []Change) error { calls++; return nil }); err != nil || calls != 0 {
		t.Errorf("Walk made %d calls, %v; want none", calls, err)
	}
	gittest.Write(t, dir, "a.txt", "a\n")
	id := gittest.Commit(t, dir, "a")
	if head, err := r.Head(ctx); err != nil || head != id {
		t.Errorf("Head = %q, %v; want %s", head, err, id)
	}
}

func TestFirstParentHistory(t *testing.T) {
	ctx := t.Context()
	dir := gittest.Init(t)
	gittest.Write(t, dir, "a.txt", "a\n")
	root := gittest.Commit(t, dir, "root")
	empty := gittest.Commit(t, dir, "")
	gittest.Git(t, dir, "checkout", "--quiet", "-b", "side")
	gittest.Write(t, dir, "dir/b c.txt", "b\n")
	side := gittest.Commit(t, dir, "side")
	gittest.Git(t, dir, "checkout", "--quiet", "main")
	gittest.Write(t, dir, "a.txt", "a2\n")
	main2 := gittest.Commit(t, dir, "main2")
	gittest.Git(t, dir, "merge", "--quiet", "--no-ff", "-m", "merge side", "side")
	merge := gittest.Git(t, dir, "rev-parse", "HEAD")
	r := open(t, dir)

	commits, err := r.Commits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range commits {
		ids = append(ids, c.ID)
	}
	if want := []string{merge, main2, empty, root}; !slices.Equal(ids, want) {
		t.Fatalf("Commits = %v, want %v (the side branch's commit is not listed)", ids, want)
	}
	if !slices.Equal(commits[0].Parents, []string{main2, side}) || len(commits[3].Parents) != 0 {
		t.Errorf("parents: merge %v, root %v", commits[0].Parents, commits[3].Parents)
	}
	if commits[0].Subject != "merge side" || commits[2].Subject != "" {
		t.Errorf("subjects: %q, %q", commits[0].Subject, commits[2].Subject)
	}
	if c := commits[3]; c.AuthorName != "Test" || c.AuthorEmail != "test@example.com" || c.AuthorTime.IsZero() {
		t.Errorf("author: %+v", c)
	}

	for _, tc := range []struct {
		id, want string
	}{
		{root, "A a.txt"},
		{empty, ""},
		{main2, "M a.txt"},
		{merge, "A dir/b c.txt"}, // against the first parent: what the merge brought in
	} {
		changes, err := r.Changes(ctx, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if got := summarize(changes); got != tc.want {
			t.Errorf("Changes(%s) = %q, want %q", tc.id, got, tc.want)
		}
	}
	rootChanges, _ := r.Changes(ctx, root)
	if !isZeroID(rootChanges[0].OldID) || rootChanges[0].OldMode != "000000" {
		t.Errorf("root commit's add has old side %s %s", rootChanges[0].OldMode, rootChanges[0].OldID)
	}

	var walked []string
	err = r.Walk(ctx, func(id string, changes []Change) error {
		walked = append(walked, id+" "+summarize(changes))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{root + " A a.txt", empty + " ", main2 + " M a.txt", merge + " A dir/b c.txt"}
	if !slices.Equal(walked, want) {
		t.Errorf("Walk = %q, want %q", walked, want)
	}

	stop := errors.New("stop")
	calls := 0
	if err := r.Walk(ctx, func(string, []Change) error { calls++; return stop }); !errors.Is(err, stop) || calls != 1 {
		t.Errorf("Walk after fn failed: %d calls, %v", calls, err)
	}
}

func TestMessageIsExact(t *testing.T) {
	dir := gittest.Init(t)
	id := gittest.Commit(t, dir, "Subject\n\nBody line.")
	got, err := open(t, dir).Message(t.Context(), id)
	if err != nil || got != "Subject\n\nBody line.\n" {
		t.Errorf("Message = %q, %v", got, err)
	}
}

func TestPatchExcludesPathsLiterally(t *testing.T) {
	dir := gittest.Init(t)
	gittest.Write(t, dir, "x*.txt", "star\n")
	gittest.Write(t, dir, "xy.txt", "y\n")
	id := gittest.Commit(t, dir, "files")
	patch, err := open(t, dir).Patch(t.Context(), id, []string{"x*.txt"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(patch)
	if !strings.Contains(s, "diff --git a/xy.txt b/xy.txt\n") || strings.Contains(s, "x*.txt") {
		t.Errorf("patch did not exclude exactly x*.txt:\n%s", s)
	}
	if !strings.Contains(s, "index 0000000000000000000000000000000000000000..") {
		t.Errorf("patch index lines are not full ids:\n%s", s)
	}
}

func TestPatchIgnoresUserDiffConfig(t *testing.T) {
	dir := gittest.Init(t)
	gittest.Write(t, dir, "f.txt", "one\n")
	gittest.Commit(t, dir, "base")
	gittest.Write(t, dir, "f.txt", "two\n")
	id := gittest.Commit(t, dir, "change")
	for _, kv := range [][2]string{
		{"diff.noprefix", "true"},
		{"diff.mnemonicPrefix", "true"},
		{"color.ui", "always"},
		{"color.diff", "always"},
		{"diff.external", "false"},
	} {
		gittest.Git(t, dir, "config", kv[0], kv[1])
	}
	patch, err := open(t, dir).Patch(t.Context(), id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(patch, []byte("diff --git a/f.txt b/f.txt\n")) || bytes.ContainsRune(patch, 0x1b) {
		t.Errorf("patch shaped by user config:\n%q", patch)
	}
}

func TestTree(t *testing.T) {
	ctx := t.Context()
	dir := gittest.Init(t)
	gittest.Write(t, dir, "sp ace/t\tab.txt", "x\n")
	gittest.Write(t, dir, "run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("run.sh", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	id := gittest.Commit(t, dir, "tree")
	r := open(t, dir)

	tree, err := r.Tree(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Entry{
		"sp ace/t\tab.txt": {Mode: "100644", ID: gittest.Git(t, dir, "rev-parse", id+":sp ace/t\tab.txt")},
		"run.sh":           {Mode: "100755", ID: gittest.Git(t, dir, "rev-parse", id+":run.sh")},
		"link":             {Mode: "120000", ID: gittest.Git(t, dir, "rev-parse", id+":link")},
	}
	if len(tree) != len(want) {
		t.Fatalf("Tree = %v, want %v", tree, want)
	}
	for p, e := range want {
		if tree[p] != e {
			t.Errorf("Tree[%q] = %v, want %v", p, tree[p], e)
		}
	}
	if empty, err := r.Tree(ctx, ""); err != nil || len(empty) != 0 {
		t.Errorf("Tree(\"\") = %v, %v", empty, err)
	}
}

func TestDirtyIgnoresUntrackedFiles(t *testing.T) {
	ctx := t.Context()
	dir := gittest.Init(t)
	gittest.Write(t, dir, "a.txt", "a\n")
	gittest.Commit(t, dir, "a")
	r := open(t, dir)

	check := func(want ...string) {
		t.Helper()
		got, err := r.Dirty(ctx)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("Dirty = %q, %v; want %q", got, err, want)
		}
	}
	check()
	gittest.Write(t, dir, "untracked.txt", "u\n")
	check()
	gittest.Write(t, dir, "a.txt", "changed\n")
	check("a.txt")
	gittest.Git(t, dir, "add", "a.txt")
	check("a.txt")
}

func TestImportBlobs(t *testing.T) {
	ctx := t.Context()
	srcDir := gittest.Init(t)
	gittest.Write(t, srcDir, "crlf.txt", "one\r\ntwo\r\n")
	id := gittest.Commit(t, srcDir, "crlf")
	blob := gittest.Git(t, srcDir, "rev-parse", id+":crlf.txt")

	tgtDir := gittest.Init(t)
	gittest.Git(t, tgtDir, "config", "core.autocrlf", "true")
	tgt := open(t, tgtDir)

	if missing, err := tgt.MissingObjects(ctx, []string{blob}); err != nil || !slices.Equal(missing, []string{blob}) {
		t.Fatalf("before import: MissingObjects = %v, %v", missing, err)
	}
	imported, err := tgt.ImportBlobs(ctx, open(t, srcDir), []string{blob})
	if err != nil || !slices.Equal(imported, []string{blob}) {
		t.Fatalf("ImportBlobs = %v, %v", imported, err)
	}
	if missing, err := tgt.MissingObjects(ctx, []string{blob}); err != nil || len(missing) != 0 {
		t.Errorf("after import: MissingObjects = %v, %v", missing, err)
	}
	if again, err := tgt.ImportBlobs(ctx, open(t, srcDir), []string{blob}); err != nil || len(again) != 0 {
		t.Errorf("second import copied %v, %v", again, err)
	}
	if status := gittest.Git(t, tgtDir, "status", "--porcelain"); status != "" {
		t.Errorf("imported blobs are visible to status: %q", status)
	}
}

// divergedPair makes a source whose second commit changes a line, and an
// unrelated target whose copy of the file changed that same line differently.
func divergedPair(t *testing.T) (src, tgt *Repo, commit string) {
	t.Helper()
	s := gittest.Init(t)
	gittest.Write(t, s, "f.txt", "l1\nl2\nl3\n")
	gittest.Commit(t, s, "base")
	gittest.Write(t, s, "f.txt", "l1\nSRC\nl3\n")
	commit = gittest.Commit(t, s, "change")
	d := gittest.Init(t)
	gittest.Write(t, d, "f.txt", "l1\nTGT\nl3\n")
	gittest.Commit(t, d, "target")
	return open(t, s), open(t, d), commit
}

func importPreimages(t *testing.T, src, tgt *Repo, commit string) {
	t.Helper()
	changes, err := src.Changes(t.Context(), commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tgt.ImportBlobs(t.Context(), src, Preimages(changes)); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewNeedsThePreimageAndTouchesNothing(t *testing.T) {
	ctx := t.Context()
	src, tgt, commit := divergedPair(t)
	patch, err := src.Patch(ctx, commit, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, _ := tgt.Head(ctx)

	// Without the preimage blob, git falls back to a straight patch and
	// refuses the conflicting edit instead of merging it.
	var ae *ApplyError
	if _, err := tgt.Preview(ctx, head, patch); !errors.As(err, &ae) {
		t.Fatalf("Preview without the preimage = %v, want *ApplyError", err)
	}

	importPreimages(t, src, tgt, commit)
	res, err := tgt.Preview(ctx, head, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Conflicts, []string{"f.txt"}) || summarize(res.Files) != "U f.txt" {
		t.Errorf("Preview = files %q, conflicts %q", summarize(res.Files), res.Conflicts)
	}
	if dirty, _ := tgt.Dirty(ctx); len(dirty) != 0 {
		t.Errorf("Preview touched the index: %v", dirty)
	}
	if got := gittest.Read(t, tgt.Dir, "f.txt"); got != "l1\nTGT\nl3\n" {
		t.Errorf("Preview touched the working tree: %q", got)
	}
}

func TestApplyConflictThenReset(t *testing.T) {
	ctx := t.Context()
	src, tgt, commit := divergedPair(t)
	patch, _ := src.Patch(ctx, commit, nil)
	head, _ := tgt.Head(ctx)
	importPreimages(t, src, tgt, commit)

	res, err := tgt.Apply(ctx, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Conflicts, []string{"f.txt"}) {
		t.Errorf("Apply conflicts = %q", res.Conflicts)
	}
	got := gittest.Read(t, tgt.Dir, "f.txt")
	if !strings.Contains(got, "<<<<<<<") || !strings.Contains(got, "SRC") || !strings.Contains(got, "TGT") {
		t.Errorf("no conflict markers:\n%s", got)
	}
	if unmerged, _ := tgt.Unmerged(ctx); !slices.Equal(unmerged, []string{"f.txt"}) {
		t.Errorf("Unmerged = %q", unmerged)
	}
	if now, _ := tgt.Head(ctx); now != head {
		t.Error("Apply moved HEAD")
	}

	if err := tgt.ResetHard(ctx); err != nil {
		t.Fatal(err)
	}
	if dirty, _ := tgt.Dirty(ctx); len(dirty) != 0 {
		t.Errorf("after reset, dirty: %v", dirty)
	}
	if got := gittest.Read(t, tgt.Dir, "f.txt"); got != "l1\nTGT\nl3\n" {
		t.Errorf("after reset, f.txt = %q", got)
	}
}

func TestApplyCleanStagesWithoutCommitting(t *testing.T) {
	ctx := t.Context()
	s := gittest.Init(t)
	gittest.Write(t, s, "f.txt", "keep\n")
	gittest.Write(t, s, "gone.txt", "bye\n")
	gittest.Commit(t, s, "base")
	gittest.Write(t, s, "f.txt", "keep\ntrailing space \n")
	gittest.Remove(t, s, "gone.txt")
	gittest.Write(t, s, "new/n.txt", "new\n")
	commit := gittest.Commit(t, s, "change")

	d := gittest.Init(t)
	gittest.Git(t, d, "config", "apply.whitespace", "error") // pinned away by applyArgs
	gittest.Write(t, d, "f.txt", "keep\n")
	gittest.Write(t, d, "gone.txt", "bye\n")
	gittest.Write(t, d, "other.txt", "target only\n")
	head := gittest.Commit(t, d, "target")

	src, tgt := open(t, s), open(t, d)
	patch, _ := src.Patch(ctx, commit, nil)
	importPreimages(t, src, tgt, commit)

	const want = "M f.txt, D gone.txt, A new/n.txt"
	pre, err := tgt.Preview(ctx, head, patch)
	if err != nil {
		t.Fatal(err)
	}
	if got := summarize(pre.Files); got != want || len(pre.Conflicts) != 0 {
		t.Errorf("Preview = %q, conflicts %q; want %q", got, pre.Conflicts, want)
	}
	res, err := tgt.Apply(ctx, patch)
	if err != nil {
		t.Fatal(err)
	}
	if got := summarize(res.Files); got != want {
		t.Errorf("Apply = %q, want %q", got, want)
	}
	if staged := gittest.Git(t, d, "diff", "--cached", "--name-status"); staged != "M\tf.txt\nD\tgone.txt\nA\tnew/n.txt" {
		t.Errorf("staged:\n%s", staged)
	}
	if now := gittest.Git(t, d, "rev-parse", "HEAD"); now != head {
		t.Error("Apply committed")
	}
	if got := gittest.Read(t, d, "f.txt"); got != "keep\ntrailing space \n" {
		t.Errorf("f.txt = %q", got)
	}
}

func TestApplyRefusalWritesNothing(t *testing.T) {
	ctx := t.Context()
	s := gittest.Init(t)
	gittest.Write(t, s, "f.txt", "l1\nl2\nl3\n")
	gittest.Write(t, s, "g.txt", "gone\n")
	gittest.Commit(t, s, "base")
	gittest.Write(t, s, "f.txt", "l1\nSRC\nl3\n")
	gittest.Remove(t, s, "g.txt")
	commit := gittest.Commit(t, s, "change f, delete g")

	d := gittest.Init(t)
	gittest.Write(t, d, "f.txt", "l1\nTGT\nl3\n") // would conflict
	gittest.Commit(t, d, "target has no g.txt")   // cannot be deleted

	src, tgt := open(t, s), open(t, d)
	patch, _ := src.Patch(ctx, commit, nil)
	importPreimages(t, src, tgt, commit)

	var ae *ApplyError
	if _, err := tgt.Apply(ctx, patch); !errors.As(err, &ae) || !strings.Contains(ae.Stderr, "g.txt") {
		t.Fatalf("Apply = %v, want an *ApplyError naming g.txt", err)
	}
	if dirty, _ := tgt.Dirty(ctx); len(dirty) != 0 {
		t.Errorf("a refused apply wrote %v", dirty)
	}
	if got := gittest.Read(t, d, "f.txt"); got != "l1\nTGT\nl3\n" {
		t.Errorf("a refused apply merged f.txt anyway: %q", got)
	}
}

func TestEmptyTargetAppliesAndResets(t *testing.T) {
	ctx := t.Context()
	s := gittest.Init(t)
	gittest.Write(t, s, "a.txt", "a\n")
	gittest.Write(t, s, "d/sub/b.txt", "b\n")
	root := gittest.Commit(t, s, "root")

	d := gittest.Init(t)
	gittest.Write(t, d, "keep.txt", "untracked\n")
	src, tgt := open(t, s), open(t, d)
	patch, err := src.Patch(ctx, root, nil)
	if err != nil {
		t.Fatal(err)
	}

	pre, err := tgt.Preview(ctx, "", patch)
	if err != nil {
		t.Fatal(err)
	}
	if got := summarize(pre.Files); got != "A a.txt, A d/sub/b.txt" {
		t.Errorf("Preview = %q", got)
	}
	if _, err := tgt.Apply(ctx, patch); err != nil {
		t.Fatal(err)
	}
	if dirty, _ := tgt.Dirty(ctx); !slices.Equal(dirty, []string{"a.txt", "d/sub/b.txt"}) {
		t.Errorf("after apply, dirty = %q", dirty)
	}

	if err := tgt.ResetHard(ctx); err != nil {
		t.Fatal(err)
	}
	if dirty, _ := tgt.Dirty(ctx); len(dirty) != 0 {
		t.Errorf("after reset, dirty = %q", dirty)
	}
	for _, p := range []string{"a.txt", "d"} {
		if gittest.Exists(t, d, p) {
			t.Errorf("after reset, %s still exists", p)
		}
	}
	if !gittest.Exists(t, d, "keep.txt") {
		t.Error("reset removed an untracked file")
	}
}

func TestSquashMsg(t *testing.T) {
	ctx := t.Context()
	r := open(t, gittest.Init(t))
	p, err := r.SquashMsgPath(ctx)
	if err != nil || p != filepath.Join(r.Dir, ".git", "SQUASH_MSG") {
		t.Fatalf("SquashMsgPath = %s, %v", p, err)
	}
	if has, _ := r.HasSquashMsg(ctx); has {
		t.Error("a fresh repository has a SQUASH_MSG")
	}
	if err := r.WriteSquashMsg(ctx, "carried\n"); err != nil {
		t.Fatal(err)
	}
	if has, _ := r.HasSquashMsg(ctx); !has || gittest.Read(t, r.Dir, ".git/SQUASH_MSG") != "carried\n" {
		t.Error("WriteSquashMsg did not write the message")
	}
	if removed, err := r.RemoveSquashMsg(ctx); !removed || err != nil {
		t.Errorf("RemoveSquashMsg = %v, %v", removed, err)
	}
	if removed, err := r.RemoveSquashMsg(ctx); removed || err != nil {
		t.Errorf("second RemoveSquashMsg = %v, %v", removed, err)
	}
}

func TestSHA256Repository(t *testing.T) {
	ctx := t.Context()
	dir := gittest.Init(t, "--object-format=sha256")
	gittest.Write(t, dir, "a.txt", "a\n")
	id := gittest.Commit(t, dir, "root")
	r := open(t, dir)
	if f, err := r.ObjectFormat(ctx); err != nil || f != "sha256" {
		t.Errorf("ObjectFormat = %q, %v", f, err)
	}
	if empty, err := r.EmptyTree(ctx); err != nil || len(empty) != 64 {
		t.Errorf("EmptyTree = %q, %v", empty, err)
	}
	if changes, err := r.Changes(ctx, id); err != nil || summarize(changes) != "A a.txt" {
		t.Errorf("Changes = %q, %v", summarize(changes), err)
	}
}
