package git

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Sknoww/floe/internal/gittest"
)

// numbered is a 20-line file, "line1" to "line20", each line passed through
// edit.
func numbered(edit func(n int, line string) string) string {
	var b strings.Builder
	for n := 1; n <= 20; n++ {
		b.WriteString(edit(n, "line"+strconv.Itoa(n)) + "\n")
	}
	return b.String()
}

// The source's f.txt has a line only it carries at 4, and the source's commit
// changes line 10. The target rewrote lines 3 to 12. The patch's context is
// three lines each side of line 10, so it never mentions line 4 — but the
// conflict spans the target's whole rewrite, and brings line 4 in.
var (
	wideBase = numbered(func(n int, line string) string {
		if n == 4 {
			return "source only"
		}
		return line
	})
	wideChanged = numbered(func(n int, line string) string {
		switch n {
		case 4:
			return "source only"
		case 10:
			return "SRC10"
		}
		return line
	})
	wideTarget = numbered(func(n int, line string) string {
		if n >= 3 && n <= 12 {
			return "T" + line
		}
		return line
	})
)

func TestPreviewConflictsOnAddAddAsApplyDoes(t *testing.T) {
	ctx := t.Context()
	s := gittest.Init(t)
	gittest.Write(t, s, "x.txt", "x\n")
	gittest.Commit(t, s, "base")
	gittest.Write(t, s, "new.txt", "the source adds\n")
	commit := gittest.Commit(t, s, "add new.txt")

	d := gittest.Init(t)
	gittest.Write(t, d, "new.txt", "the target has\n")
	head := gittest.Commit(t, d, "target")

	src, tgt := open(t, s), open(t, d)
	patch, err := src.Patch(ctx, commit, nil)
	if err != nil {
		t.Fatal(err)
	}
	pre, err := tgt.Preview(ctx, head, patch)
	if err != nil {
		t.Fatalf("Preview = %v; the real apply conflicts", err)
	}
	if !slices.Equal(pre.Conflicts, []string{"new.txt"}) {
		t.Errorf("Preview conflicts = %q", pre.Conflicts)
	}
	if dirty, _ := tgt.Dirty(ctx); len(dirty) != 0 {
		t.Errorf("Preview touched the target: %q", dirty)
	}
	if got := pre.Landed["new.txt"]; !slices.Contains(got, "the source adds") || slices.Contains(got, "the target has") {
		t.Errorf("Landed[new.txt] = %q", got)
	}

	res, err := tgt.Apply(ctx, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Conflicts, pre.Conflicts) {
		t.Errorf("Apply conflicts = %q, Preview's = %q", res.Conflicts, pre.Conflicts)
	}
}

func TestPreviewReportsWhatLands(t *testing.T) {
	ctx := t.Context()
	s := gittest.Init(t)
	gittest.Write(t, s, "f.txt", wideBase)
	gittest.Write(t, s, "clean.txt", "keep\n")
	gittest.Write(t, s, "gone.txt", "bye\n")
	gittest.Write(t, s, "b.bin", "\x00base")
	gittest.Commit(t, s, "base")
	gittest.Write(t, s, "f.txt", wideChanged)
	gittest.Write(t, s, "clean.txt", "keep\nadded\n")
	gittest.Remove(t, s, "gone.txt")
	gittest.Write(t, s, "b.bin", "\x00changed")
	gittest.Write(t, s, "new.txt", "n1\nn2\n")
	commit := gittest.Commit(t, s, "change")

	d := gittest.Init(t)
	gittest.Write(t, d, "f.txt", wideTarget)
	gittest.Write(t, d, "clean.txt", "keep\n")
	gittest.Write(t, d, "gone.txt", "bye\n")
	gittest.Write(t, d, "b.bin", "\x00base")
	head := gittest.Commit(t, d, "target")

	src, tgt := open(t, s), open(t, d)
	patch, err := src.Patch(ctx, commit, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(patch), "source only") {
		t.Fatalf("the fixture's patch mentions line 4:\n%s", patch)
	}
	importPreimages(t, src, tgt, commit)
	pre, err := tgt.Preview(ctx, head, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pre.Conflicts, []string{"f.txt"}) {
		t.Fatalf("Preview conflicts = %q", pre.Conflicts)
	}

	for path, want := range map[string][]string{"clean.txt": {"added"}, "new.txt": {"n1", "n2"}} {
		if got := pre.Landed[path]; !slices.Equal(got, want) {
			t.Errorf("Landed[%s] = %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{"gone.txt", "b.bin"} {
		if got, ok := pre.Landed[path]; ok {
			t.Errorf("Landed[%s] = %q, want nothing", path, got)
		}
	}
	f := pre.Landed["f.txt"]
	if !slices.Contains(f, "source only") || !slices.Contains(f, "SRC10") {
		t.Errorf("Landed[f.txt] misses the conflict's source lines: %q", f)
	}
	for _, line := range f {
		if strings.HasPrefix(line, "Tline") {
			t.Errorf("Landed[f.txt] has the target's own line %q", line)
		}
	}

	// Every line the real apply writes that the target did not have is one
	// the preview reported.
	if _, err := tgt.Apply(ctx, patch); err != nil {
		t.Fatal(err)
	}
	had := strings.Split(wideTarget, "\n")
	for _, line := range strings.Split(strings.TrimSuffix(gittest.Read(t, d, "f.txt"), "\n"), "\n") {
		if !slices.Contains(had, line) && !slices.Contains(f, line) {
			t.Errorf("apply wrote %q, which the preview did not report", line)
		}
	}
}

func TestPreviewLandedFollowsAMergeDriver(t *testing.T) {
	ctx := t.Context()
	s := gittest.Init(t)
	gittest.Write(t, s, "f.txt", wideBase)
	gittest.Commit(t, s, "base")
	gittest.Write(t, s, "f.txt", wideChanged)
	commit := gittest.Commit(t, s, "change line 10")

	d := gittest.Init(t)
	gittest.Write(t, d, "f.txt", wideTarget)
	gittest.Write(t, d, ".gitattributes", "f.txt merge=union\n")
	head := gittest.Commit(t, d, "target merges f.txt by union")

	src, tgt := open(t, s), open(t, d)
	patch, err := src.Patch(ctx, commit, nil)
	if err != nil {
		t.Fatal(err)
	}
	importPreimages(t, src, tgt, commit)
	pre, err := tgt.Preview(ctx, head, patch)
	if err != nil {
		t.Fatal(err)
	}
	// The union driver merges cleanly — and keeps the source's side whole.
	if len(pre.Conflicts) != 0 {
		t.Errorf("Preview conflicts = %q, want none", pre.Conflicts)
	}
	if got := pre.Landed["f.txt"]; !slices.Contains(got, "source only") {
		t.Errorf("Landed[f.txt] = %q, want the source's line 4", got)
	}
}
