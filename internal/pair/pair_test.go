package pair

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, err := Root(); err != nil || got != "/xdg/floe" {
		t.Errorf("with XDG_CONFIG_HOME: Root = %q, %v", got, err)
	}
	// The XDG spec ignores a relative value.
	t.Setenv("XDG_CONFIG_HOME", "relative")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Root(); err != nil || got != filepath.Join(home, ".config", "floe") {
		t.Errorf("without XDG_CONFIG_HOME: Root = %q, %v", got, err)
	}
}

func TestFileName(t *testing.T) {
	name := FileName("/work/app-internal", "/work/app-public")
	prefix := "app-internal--app-public-"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") || len(name) != len(prefix)+8+len(".json") {
		t.Errorf("FileName = %q", name)
	}
	for _, other := range [][2]string{
		{"/elsewhere/app-internal", "/work/app-public"}, // same names, different place
		{"/work/app-public", "/work/app-internal"},      // the pair reversed
	} {
		if got := FileName(other[0], other[1]); got == name {
			t.Errorf("FileName(%q, %q) = %q, the same as another pair's", other[0], other[1], got)
		}
	}
}

func TestRememberCreatesThenKeeps(t *testing.T) {
	root := t.TempDir()
	src, tgt := "/work/app-internal", "/work/app-public"
	opened := time.Date(2026, 9, 11, 17, 30, 0, 500, time.FixedZone("MDT", -6*3600))

	c, err := Remember(root, src, tgt, opened)
	if err != nil {
		t.Fatal(err)
	}
	path := Path(root, src, tgt)
	if want := time.Date(2026, 9, 11, 23, 30, 0, 0, time.UTC); !c.LastOpened.Equal(want) || c.LastOpened.Location() != time.UTC {
		t.Errorf("LastOpened = %v, want %v", c.LastOpened, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"source\": \"/work/app-internal\",\n  \"target\": \"/work/app-public\",\n  \"exclude\": [],\n  \"guard\": [],\n  \"lastOpened\": \"2026-09-11T23:30:00Z\"\n}\n"; string(raw) != want {
		t.Errorf("new file:\n%s\nwant:\n%s", raw, want)
	}

	c.Exclude = []string{"README.md", ".github/**"}
	c.Guard = []string{"(?i)acme corp", `<internal>`}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), `"<internal>"`) {
		t.Errorf("a guard pattern was escaped:\n%s", raw)
	}

	c, err = Remember(root, src, tgt, opened.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Exclude, []string{"README.md", ".github/**"}) || len(c.Guard) != 2 || !c.LastOpened.Equal(opened.Add(time.Hour).Truncate(time.Second)) {
		t.Errorf("reopened: %+v", c)
	}

	// A config Load would refuse is never written.
	c.Exclude = []string{"/README.md"}
	if err := c.Save(path); err == nil || !strings.Contains(err.Error(), "exclude[0]") {
		t.Errorf("Save with a dead pattern = %v", err)
	}
	if _, err := Load(path); err != nil {
		t.Errorf("a refused save damaged the file: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "pairs", "*.tmp*")); len(leftovers) > 0 {
		t.Errorf("temp files left behind: %q", leftovers)
	}
}

func TestRememberRefusesAFileNamingAnotherPair(t *testing.T) {
	root := t.TempDir()
	path := Path(root, "/a", "/b")
	if err := (&Config{Source: "/moved", Target: "/b"}).Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Remember(root, "/a", "/b", time.Now()); err == nil || !strings.Contains(err.Error(), "/moved") {
		t.Errorf("Remember = %v, want an error naming the file's pair", err)
	}
}

func TestLoadIsStrict(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, body, want string }{
		{"unknown field", `{"source":"/a","target":"/b","excludes":["README.md"]}`, `unknown field "excludes"`},
		{"wrong type", `{"source":"/a","target":"/b","exclude":"README.md"}`, "exclude"},
		{"trailing data", `{"source":"/a","target":"/b"} {}`, "after the JSON object"},
		{"missing target", `{"source":"/a"}`, `"target" is missing`},
		{"relative source", `{"source":"a","target":"/b"}`, `"source"`},
		{"dead exclusion", `{"source":"/a","target":"/b","exclude":["README.md","docs/"]}`, "exclude[1]"},
		{"invalid glob", `{"source":"/a","target":"/b","exclude":["a["]}`, "exclude[0]"},
		{"invalid guard", `{"source":"/a","target":"/b","guard":["(?i)ok","[a"]}`, "guard[1]"},
		{"empty guard", `{"source":"/a","target":"/b","guard":[""]}`, "guard[0]"},
	} {
		path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".json")
		if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Load = %v, want an error naming the file and containing %q", tc.name, err, tc.want)
		}
	}
}

func TestCheckExclude(t *testing.T) {
	for _, p := range []string{"README.md", ".github/**", "**/*.pem", "docs", `a\*b`, "{cmd,pkg}/**"} {
		if err := CheckExclude(p); err != nil {
			t.Errorf("CheckExclude(%q) = %v", p, err)
		}
	}
	for p, want := range map[string]string{
		"":           "empty",
		"/":          "empty",
		"/README.md": `write "README.md"`,
		"docs/":      `write "docs/**"`,
		"a//b":       "empty path segment",
		"./x":        `"." segments`,
		"a/../b":     `".." segments`,
		"a[":         "not a valid glob",
	} {
		if err := CheckExclude(p); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckExclude(%q) = %v, want an error containing %q", p, err, want)
		}
	}
}

func TestExcluded(t *testing.T) {
	excluded, err := (&Config{Exclude: []string{"README.md", ".github/**", "**/*.pem"}}).Excluded()
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"README.md":                true,
		"docs/README.md":           false, // anchored at the top level
		".github/workflows/ci.yml": true,
		"certs/dev.pem":            true,
		"dev.pem":                  true,
		"app.go":                   false,
	} {
		if got := excluded(path); got != want {
			t.Errorf("excluded(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestCompiledGuard(t *testing.T) {
	guard, err := (&Config{Guard: []string{"(?i)acme corp", `internal\.example\.com`}}).CompiledGuard()
	if err != nil {
		t.Fatal(err)
	}
	if !guard[0].MatchString("calls the ACME Corp API") || guard[1].MatchString("internalXexampleXcom") {
		t.Error("guard patterns compiled wrongly")
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	if pairs, err := List(root); err != nil || pairs != nil {
		t.Errorf("List with no pairs directory = %v, %v", pairs, err)
	}

	src, tgt := t.TempDir(), t.TempDir()
	moved := filepath.Join(t.TempDir(), "moved-away")
	opened := time.Date(2026, 9, 11, 17, 30, 0, 0, time.UTC)
	if _, err := Remember(root, src, tgt, opened); err != nil {
		t.Fatal(err)
	}
	if _, err := Remember(root, moved, tgt, opened.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	pairs := filepath.Join(root, "pairs")
	for name, body := range map[string]string{
		"broken.json":             "{",
		"notes.txt":               "not a pair",
		"x--y-00000000.json.tmp1": "{",
	} {
		if err := os.WriteFile(filepath.Join(pairs, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("List = %d entries, want 3: %+v", len(got), got)
	}
	if got[0].Err != nil || got[0].Config.Source != moved || !slices.Equal(got[0].Missing, []string{moved}) {
		t.Errorf("newest: %+v", got[0])
	}
	if got[1].Err != nil || got[1].Config.Source != src || got[1].Missing != nil {
		t.Errorf("older: %+v", got[1])
	}
	if got[2].Err == nil || filepath.Base(got[2].Path) != "broken.json" {
		t.Errorf("broken file: %+v", got[2])
	}
}
