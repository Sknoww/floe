package transfer

import (
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/Sknoww/floe/internal/git"
)

func TestHeaderPath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`a/f.txt b/f.txt`, "f.txt"},
		{`a/x b/y b/x b/y`, "x b/y"},
		{`a/sp ace.txt b/sp ace.txt`, "sp ace.txt"},
		{`"a/t\tab.txt" "b/t\tab.txt"`, "t\tab.txt"},
		{`"a/\303\251.txt" "b/\303\251.txt"`, "é.txt"},
		{`"a/q\"uote" "b/q\"uote"`, `q"uote`},
	} {
		got, err := headerPath(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("headerPath(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{`a/f.txt b/g.txt`, `"a/unterminated`, `x`} {
		if _, err := headerPath(bad); err == nil {
			t.Errorf("headerPath(%q) accepted it", bad)
		}
	}
}

func TestSplitPatch(t *testing.T) {
	const patch = `diff --git a/t.txt b/t.txt
index 1111111111111111111111111111111111111111..2222222222222222222222222222222222222222 100644
--- a/t.txt
+++ b/t.txt
@@ -1,2 +1,3 @@
 context
-removed
+++ an added line that looks like a header
+added
diff --git a/bin.dat b/bin.dat
new file mode 100644
index 0000000000000000000000000000000000000000..3333333333333333333333333333333333333333
GIT binary patch
literal 4
LcmZQzWMT#Y01f~L

literal 0
HcmV?d00001

`
	files, err := splitPatch([]byte(patch))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("splitPatch found %d files, want 2", len(files))
	}
	if f := files[0]; f.Path != "t.txt" || f.Binary || !slices.Equal(f.Added, []git.Line{{No: 2, Text: "++ an added line that looks like a header"}, {No: 3, Text: "added"}}) {
		t.Errorf("text file: %+v", f)
	}
	if f := files[1]; f.Path != "bin.dat" || !f.Binary || len(f.Added) != 0 {
		t.Errorf("binary file: %+v", f)
	}
}

func TestGuardMatchesAddedLinesAndTheMessage(t *testing.T) {
	guard := []*regexp.Regexp{regexp.MustCompile(`(?i)acme corp`), regexp.MustCompile(`secret\nplan`)}
	files := []filePatch{{Path: "a.go", Added: []git.Line{{No: 7, Text: "// for Acme Corp, and acme corp"}, {No: 8, Text: "fine"}}}}
	message := "Subject\n\nsecret\nplan for acme corp\n"
	got := guardMatches(guard, files, message)
	want := []GuardMatch{
		{Pattern: "(?i)acme corp", Path: "a.go", LineNo: 7, Line: "// for Acme Corp, and acme corp", Spans: [][2]int{{7, 16}, {22, 31}}},
		{Pattern: "(?i)acme corp", LineNo: 4, Line: "plan for acme corp", Spans: [][2]int{{9, 18}}},
		// A match that runs on past its line is marked to the line's end.
		{Pattern: `secret\nplan`, LineNo: 3, Line: "secret", Spans: [][2]int{{0, 6}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("guardMatches = %+v, want %+v", got, want)
	}
}
