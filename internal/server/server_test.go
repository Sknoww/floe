package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Sknoww/floe/internal/git"
	"github.com/Sknoww/floe/internal/gittest"
	"github.com/Sknoww/floe/internal/pair"
)

func TestMain(m *testing.M) { gittest.Main(m) }

// harness is one launch of floe against a config root, driven over real HTTP.
type harness struct {
	t   *testing.T
	s   *Server
	url string
}

func launch(t *testing.T, root string) *harness {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	s, err := New(Options{
		Root:   root,
		Addr:   ts.Listener.Addr(),
		Assets: fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>floe</title>")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) }
	ts.Config.Handler = s.Handler()
	ts.Start()
	t.Cleanup(ts.Close)
	return &harness{t: t, s: s, url: ts.URL}
}

// call sends an API request carrying the token, fails the test unless the
// response has the wanted status, and decodes it into out.
func (h *harness) call(method, path string, body any, want int, out any) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.url+path, rd)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set(TokenHeader, h.s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	if resp.StatusCode != want {
		h.t.Fatalf("%s %s = %d, want %d: %s", method, path, resp.StatusCode, want, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("%s %s: %v: %s", method, path, err, raw)
		}
	}
}

type errorResponse struct {
	Error apiError `json:"error"`
}

// fixture remembers a pair under root, as `floe <source> <target>` does: a
// source with a base commit and a change on top, and an unrelated target
// holding the base, so the change applies cleanly. README.md never crosses.
func fixture(t *testing.T, root string) (id, src, tgt, commit string) {
	t.Helper()
	src = gittest.Init(t)
	gittest.Write(t, src, "f.txt", "l1\nl2\nl3\n")
	gittest.Write(t, src, "README.md", "internal\n")
	gittest.Commit(t, src, "Base")
	gittest.Write(t, src, "f.txt", "l1\nSRC\nl3\n")
	gittest.Write(t, src, "README.md", "internal, changed\n")
	gittest.Write(t, src, "new.txt", "new\n")
	commit = gittest.Commit(t, src, "Change f\n\nWith a body.")

	tgt = gittest.Init(t)
	gittest.Write(t, tgt, "f.txt", "l1\nl2\nl3\n")
	gittest.Write(t, tgt, "README.md", "public\n")
	gittest.Commit(t, tgt, "Target")

	src, tgt = resolve(t, src), resolve(t, tgt)
	c, err := pair.Remember(root, src, tgt, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c.Exclude = []string{"README.md"}
	if err := c.Save(pair.Path(root, src, tgt)); err != nil {
		t.Fatal(err)
	}
	return pair.ID(src, tgt), src, tgt, commit
}

// resolve is a repository's top level as floe identifies it.
func resolve(t *testing.T, dir string) string {
	t.Helper()
	r, err := git.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r.Dir
}

func summarize(changes []changeJSON) string {
	var parts []string
	for _, c := range changes {
		parts = append(parts, c.Status+" "+c.Path)
	}
	return strings.Join(parts, ", ")
}

func TestNewRefusesAnyInterfaceButLoopback(t *testing.T) {
	if _, err := New(Options{Addr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 8080}}); err == nil {
		t.Error("New accepted a listener on a network interface")
	}
}

func TestRequestsMustBeAddressedToFloeAndCarryTheToken(t *testing.T) {
	h := launch(t, t.TempDir())
	_, port, err := net.SplitHostPort(h.s.host)
	if err != nil {
		t.Fatal(err)
	}
	none := func(*http.Request) {}
	token := func(r *http.Request) { r.Header.Set(TokenHeader, h.s.token) }
	for _, tc := range []struct {
		name, path string
		edit       func(*http.Request)
		want       int
	}{
		{"the page, without the token", "/", none, http.StatusOK},
		{"a missing asset", "/nope.js", none, http.StatusNotFound},
		{"the API with the token", "/api/pairs", token, http.StatusOK},
		{"the API without the token", "/api/pairs", none, http.StatusUnauthorized},
		{"the API with a guessed token", "/api/pairs", func(r *http.Request) { r.Header.Set(TokenHeader, "guess") }, http.StatusUnauthorized},
		{"the API from its own origin", "/api/pairs", func(r *http.Request) {
			token(r)
			r.Header.Set("Origin", "http://"+h.s.host)
		}, http.StatusOK},
		{"the API from another origin", "/api/pairs", func(r *http.Request) {
			token(r)
			r.Header.Set("Origin", "http://evil.example")
		}, http.StatusForbidden},
		{"the API under a rebound name", "/api/pairs", func(r *http.Request) {
			token(r)
			r.Host = "evil.example:" + port
		}, http.StatusMisdirectedRequest},
		{"the page under another name", "/", func(r *http.Request) { r.Host = "localhost:" + port }, http.StatusMisdirectedRequest},
		{"a route the API lacks", "/api/nope", token, http.StatusNotFound},
	} {
		req, err := http.NewRequest(http.MethodGet, h.url+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		tc.edit(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}
}

func TestPairsAndSettings(t *testing.T) {
	root := t.TempDir()
	id, src, tgt, _ := fixture(t, root)
	h := launch(t, root)

	var list struct {
		Home  string           `json:"home"`
		Pairs []rememberedJSON `json:"pairs"`
	}
	h.call("GET", "/api/pairs", nil, http.StatusOK, &list)
	if len(list.Pairs) != 1 || list.Pairs[0].ID != id || list.Pairs[0].Source == nil || list.Pairs[0].Source.Path != src {
		t.Errorf("pairs = %+v", list.Pairs)
	}
	// The page needs the home directory to abbreviate a path to "~/…".
	if home, err := os.UserHomeDir(); err == nil && list.Home != home {
		t.Errorf("home = %q, want %q", list.Home, home)
	}

	var p pairJSON
	h.call("POST", "/api/pairs/"+id+"/open", nil, http.StatusOK, &p)
	if p.ID != id || p.Target.Path != tgt || !slices.Equal(p.Exclude, []string{"README.md"}) || !p.LastOpened.Equal(h.s.now()) {
		t.Errorf("opened pair = %+v", p)
	}
	h.call("GET", "/api/pairs/nope-00000000", nil, http.StatusNotFound, nil)

	// A pattern is checked as it is typed, with floe's words and suggestion.
	var check patternJSON
	h.call("POST", "/api/patterns/check", patternRequest{Kind: "exclude", Pattern: "docs/"}, http.StatusOK, &check)
	if !strings.Contains(check.Error, "can never match") || check.Suggestion != "docs/**" {
		t.Errorf("check docs/ = %+v", check)
	}
	check = patternJSON{}
	h.call("POST", "/api/patterns/check", patternRequest{Kind: "guard", Pattern: "status.example.com (beta", Literal: true}, http.StatusOK, &check)
	if check.Error != "" || check.Pattern != `status\.example\.com \(beta` {
		t.Errorf("literal check = %+v", check)
	}
	h.call("POST", "/api/patterns/check", patternRequest{Kind: "guard", Pattern: "(?i)acme (corp"}, http.StatusOK, &check)
	if !strings.Contains(check.Error, "missing closing )") {
		t.Errorf("invalid guard check = %+v", check)
	}
	h.call("POST", "/api/patterns/check", patternRequest{Kind: "exclude", Pattern: "x", Literal: true}, http.StatusBadRequest, nil)

	// A save with any invalid pattern saves nothing.
	var refused errorResponse
	h.call("PUT", "/api/pairs/"+id+"/settings", settingsRequest{Exclude: []string{"README.md", "docs/"}}, http.StatusUnprocessableEntity, &refused)
	if refused.Error.Code != "invalid_settings" || !strings.Contains(refused.Error.Message, "exclude[1]") {
		t.Errorf("invalid save = %+v", refused.Error)
	}
	h.call("PUT", "/api/pairs/"+id+"/settings", settingsRequest{Exclude: []string{"docs/**"}, Guard: []string{"(?i)acme corp"}}, http.StatusOK, &p)
	c, err := pair.Load(pair.Path(root, src, tgt))
	if err != nil || !slices.Equal(c.Exclude, []string{"docs/**"}) || !slices.Equal(c.Guard, []string{"(?i)acme corp"}) || !slices.Equal(p.Guard, c.Guard) {
		t.Errorf("saved settings = %+v, %v; response %+v", c, err, p)
	}
}

func TestTransferStagesAndOutlivesARestart(t *testing.T) {
	root := t.TempDir()
	id, src, tgt, commit := fixture(t, root)
	base := gittest.Git(t, src, "rev-parse", commit+"^")
	head := gittest.Git(t, tgt, "rev-parse", "HEAD")
	h := launch(t, root)
	api := "/api/pairs/" + id

	var commits struct {
		Branch  string       `json:"branch"`
		Commits []commitJSON `json:"commits"`
	}
	h.call("GET", api+"/commits", nil, http.StatusOK, &commits)
	if commits.Branch != "main" || len(commits.Commits) != 2 || commits.Commits[0].ID != commit || commits.Commits[0].Subject != "Change f" {
		t.Errorf("commits = %+v", commits)
	}

	var detail struct {
		Message string     `json:"message"`
		Files   []fileJSON `json:"files"`
	}
	h.call("GET", api+"/commits/"+commit, nil, http.StatusOK, &detail)
	var files []string
	for _, f := range detail.Files {
		files = append(files, fmt.Sprintf("%s %s +%d -%d excluded=%v", f.Status, f.Path, f.Added, f.Deleted, f.Excluded))
	}
	if want := []string{"M README.md +1 -1 excluded=true", "M f.txt +1 -1 excluded=false", "A new.txt +1 -0 excluded=false"}; !slices.Equal(files, want) || detail.Message != "Change f\n\nWith a body.\n" {
		t.Errorf("commit files = %q, message %q", files, detail.Message)
	}
	var diff struct {
		Diff string `json:"diff"`
	}
	h.call("GET", api+"/commits/"+commit+"/diff?path=f.txt", nil, http.StatusOK, &diff)
	if !strings.Contains(diff.Diff, "-l2\n+SRC\n") || strings.Contains(diff.Diff, "new.txt") {
		t.Errorf("diff of f.txt:\n%s", diff.Diff)
	}
	h.call("GET", api+"/commits/HEAD", nil, http.StatusBadRequest, nil)

	var before targetJSON
	h.call("GET", api+"/target", nil, http.StatusOK, &before)
	if before.Head != head || before.Branch != "main" || before.Position.Match != base || len(before.Dirty) != 0 || before.Message != nil || before.Transfer != nil {
		t.Errorf("target before = %+v", before)
	}

	var pv previewJSON
	h.call("POST", api+"/preview", previewRequest{Commit: commit}, http.StatusOK, &pv)
	if pv.ID == "" || pv.TargetHead != head || summarize(pv.Result.Files) != "M f.txt, A new.txt" || len(pv.Result.Conflicts) != 0 {
		t.Errorf("preview = %+v", pv)
	}
	if status := gittest.Git(t, tgt, "status", "--porcelain"); status != "" {
		t.Errorf("the preview touched the target: %q", status)
	}

	var gone errorResponse
	h.call("POST", api+"/apply", applyRequest{Preview: "not-this-one"}, http.StatusConflict, &gone)
	if gone.Error.Code != "preview_gone" {
		t.Errorf("apply of an unknown preview = %+v", gone.Error)
	}
	var applied appliedJSON
	h.call("POST", api+"/apply", applyRequest{Preview: pv.ID}, http.StatusOK, &applied)
	if applied.Transfer == nil || applied.Transfer.Commit != commit || applied.Transfer.TargetHead != head {
		t.Errorf("applied = %+v", applied)
	}
	if staged := gittest.Git(t, tgt, "diff", "--cached", "--name-status"); staged != "M\tf.txt\nA\tnew.txt" {
		t.Errorf("staged:\n%s", staged)
	}
	h.call("POST", api+"/apply", applyRequest{Preview: pv.ID}, http.StatusConflict, nil)

	// A new launch still knows the staged transfer for floe's.
	h = launch(t, root)
	var after targetJSON
	h.call("GET", api+"/target", nil, http.StatusOK, &after)
	if after.Transfer == nil || after.Transfer.Commit != commit || after.Message == nil || *after.Message != "Change f\n\nWith a body.\n" ||
		summarize(after.Dirty) != "M f.txt, A new.txt" {
		t.Errorf("target after a restart = %+v", after)
	}

	h.call("POST", api+"/discard", nil, http.StatusOK, nil)
	if status := gittest.Git(t, tgt, "status", "--porcelain"); status != "" || gittest.Exists(t, tgt, ".git/SQUASH_MSG") {
		t.Errorf("after discard: status %q, SQUASH_MSG left: %v", status, gittest.Exists(t, tgt, ".git/SQUASH_MSG"))
	}
	if rec, err := pair.LoadTransfer(root, src, tgt); rec != nil || err != nil {
		t.Errorf("after discard: record %+v, %v", rec, err)
	}
	h.call("POST", api+"/discard", nil, http.StatusConflict, &gone)
	if gone.Error.Code != "no_transfer" {
		t.Errorf("second discard = %+v", gone.Error)
	}
}

func TestDiscardNeverTouchesTheUsersOwnWork(t *testing.T) {
	root := t.TempDir()
	id, src, tgt, commit := fixture(t, root)
	h := launch(t, root)
	api := "/api/pairs/" + id

	// Changes with no record are the user's.
	gittest.Write(t, tgt, "f.txt", "the user's own work\n")
	var refused errorResponse
	h.call("POST", api+"/discard", nil, http.StatusConflict, &refused)
	if refused.Error.Code != "no_transfer" || gittest.Read(t, tgt, "f.txt") != "the user's own work\n" {
		t.Errorf("discard over the user's work = %+v", refused.Error)
	}
	gittest.Git(t, tgt, "checkout", "--", "f.txt")

	// A transfer the user has committed is theirs too, and so is what follows.
	var pv previewJSON
	h.call("POST", api+"/preview", previewRequest{Commit: commit}, http.StatusOK, &pv)
	h.call("POST", api+"/apply", applyRequest{Preview: pv.ID}, http.StatusOK, nil)
	gittest.Git(t, tgt, "commit", "--quiet", "--no-edit")
	gittest.Write(t, tgt, "f.txt", "more of the user's work\n")
	var target targetJSON
	h.call("GET", api+"/target", nil, http.StatusOK, &target)
	if target.Transfer != nil {
		t.Errorf("a committed transfer still counts: %+v", target.Transfer)
	}
	if rec, _ := pair.LoadTransfer(root, src, tgt); rec != nil {
		t.Error("the record outlived the commit")
	}
	h.call("POST", api+"/discard", nil, http.StatusConflict, &refused)
	if refused.Error.Code != "no_transfer" || gittest.Read(t, tgt, "f.txt") != "more of the user's work\n" {
		t.Errorf("discard after the commit = %+v", refused.Error)
	}
}

func TestConflictThroughTheAPI(t *testing.T) {
	root := t.TempDir()
	id, _, tgt, commit := fixture(t, root)
	gittest.Write(t, tgt, "f.txt", "l1\nTGT\nl3\n")
	gittest.Commit(t, tgt, "Target edits the same line")
	h := launch(t, root)
	api := "/api/pairs/" + id

	var pv previewJSON
	h.call("POST", api+"/preview", previewRequest{Commit: commit}, http.StatusOK, &pv)
	if !slices.Equal(pv.Result.Conflicts, []string{"f.txt"}) || !strings.Contains(pv.Result.Conflicted["f.txt"], "<<<<<<< ours\nTGT\n") {
		t.Errorf("preview = %+v", pv.Result)
	}
	h.call("POST", api+"/apply", applyRequest{Preview: pv.ID}, http.StatusOK, nil)

	var file struct {
		Content string `json:"content"`
		Binary  bool   `json:"binary"`
	}
	h.call("GET", api+"/conflict?path=f.txt", nil, http.StatusOK, &file)
	if file.Content != gittest.Read(t, tgt, "f.txt") || file.Content != pv.Result.Conflicted["f.txt"] || file.Binary {
		t.Errorf("conflict = %q; on disk %q; previewed %q", file.Content, gittest.Read(t, tgt, "f.txt"), pv.Result.Conflicted["f.txt"])
	}
	h.call("GET", api+"/conflict?path=new.txt", nil, http.StatusNotFound, nil)
	h.call("GET", api+"/conflict?path=../../etc/passwd", nil, http.StatusNotFound, nil)

	var target targetJSON
	h.call("GET", api+"/target", nil, http.StatusOK, &target)
	if target.Transfer == nil || !slices.Equal(target.Conflicts, []string{"f.txt"}) || summarize(target.Dirty) != "U f.txt, A new.txt" {
		t.Errorf("target on a conflict = %+v", target)
	}
	h.call("POST", api+"/discard", nil, http.StatusOK, nil)
	if status := gittest.Git(t, tgt, "status", "--porcelain"); status != "" {
		t.Errorf("after discard: %q", status)
	}
}

func TestRefusalsThroughTheAPI(t *testing.T) {
	root := t.TempDir()
	id, src, tgt, commit := fixture(t, root)
	h := launch(t, root)
	api := "/api/pairs/" + id

	// The guard refuses, naming where it matched and what.
	h.call("PUT", api+"/settings", settingsRequest{Exclude: []string{"README.md"}, Guard: []string{"R"}}, http.StatusOK, nil)
	var refused errorResponse
	h.call("POST", api+"/preview", previewRequest{Commit: commit}, http.StatusConflict, &refused)
	want := []matchJSON{{
		Pattern: "R", Path: "f.txt", LineNo: 2, Line: "SRC",
		Parts: []partJSON{{Text: "S"}, {Text: "R", Matched: true}, {Text: "C"}},
	}}
	if refused.Error.Code != "guard" || !reflect.DeepEqual(refused.Error.Matches, want) {
		t.Errorf("guard refusal = %+v", refused.Error)
	}

	// Unticked, the matching file stays behind and the rest crosses.
	var pv previewJSON
	h.call("POST", api+"/preview", previewRequest{Commit: commit, Skip: []string{"f.txt"}}, http.StatusOK, &pv)
	if summarize(pv.Result.Files) != "A new.txt" {
		t.Errorf("preview with f.txt unticked = %+v", pv.Result)
	}

	// A preview the settings changed after is not applied.
	h.call("PUT", api+"/settings", settingsRequest{Exclude: []string{"README.md", "new.txt"}, Guard: []string{"R"}}, http.StatusOK, nil)
	h.call("POST", api+"/apply", applyRequest{Preview: pv.ID}, http.StatusConflict, &refused)
	if refused.Error.Code != "preview_gone" || !strings.Contains(refused.Error.Message, "settings changed") {
		t.Errorf("apply after a settings change = %+v", refused.Error)
	}

	// Nor is one the target's HEAD moved after, and its record goes with it.
	h.call("PUT", api+"/settings", settingsRequest{Exclude: []string{"README.md"}}, http.StatusOK, nil)
	h.call("POST", api+"/preview", previewRequest{Commit: commit}, http.StatusOK, &pv)
	gittest.Write(t, tgt, "later.txt", "moved on\n")
	gittest.Commit(t, tgt, "Target moves")
	h.call("POST", api+"/apply", applyRequest{Preview: pv.ID}, http.StatusConflict, &refused)
	if refused.Error.Code != "stale" {
		t.Errorf("apply after HEAD moved = %+v", refused.Error)
	}
	if rec, err := pair.LoadTransfer(root, src, tgt); rec != nil || err != nil {
		t.Errorf("a refused apply left its record: %+v, %v", rec, err)
	}

	// A target with changes refuses the preview, naming them.
	gittest.Write(t, tgt, "f.txt", "uncommitted\n")
	h.call("POST", api+"/preview", previewRequest{Commit: commit}, http.StatusConflict, &refused)
	if refused.Error.Code != "dirty" || !slices.Equal(refused.Error.Paths, []string{"f.txt"}) {
		t.Errorf("preview into a dirty target = %+v", refused.Error)
	}
}
