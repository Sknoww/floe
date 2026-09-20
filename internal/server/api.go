package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/Sknoww/floe/internal/git"
	"github.com/Sknoww/floe/internal/pair"
	"github.com/Sknoww/floe/internal/transfer"
)

// config reads the pair a request names. It is read again on every request, so
// a hand edit to the file applies at once.
func (s *Server) config(r *http.Request) (*pair.Config, error) {
	id := r.PathValue("pair")
	c, err := pair.LoadID(s.root, id)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, &apiError{Status: http.StatusNotFound, Code: "pair_not_found", Message: fmt.Sprintf("floe remembers no pair named %q", id)}
	case err != nil:
		return nil, &apiError{Status: http.StatusUnprocessableEntity, Code: "pair_config", Message: err.Error()}
	}
	return c, nil
}

// open reads the pair a request names and opens its repositories, once a
// launch.
func (s *Server) open(r *http.Request) (*transfer.Pair, *pair.Config, error) {
	c, err := s.config(r)
	if err != nil {
		return nil, nil, err
	}
	id := pair.ID(c.Source, c.Target)
	s.mu.Lock()
	p := s.opened[id]
	s.mu.Unlock()
	if p != nil {
		return p, c, nil
	}
	if p, err = transfer.Open(r.Context(), c.Source, c.Target); err != nil {
		return nil, nil, &apiError{Status: http.StatusConflict, Code: "pair_unavailable", Message: err.Error()}
	}
	s.mu.Lock()
	s.opened[id] = p
	s.mu.Unlock()
	return p, c, nil
}

// commitID reads the commit a request names, which must be a full object id.
func commitID(r *http.Request) (string, error) {
	id := r.PathValue("commit")
	if !git.IsObjectID(id) {
		return "", &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: fmt.Sprintf("%q is not a full commit id", id)}
	}
	return id, nil
}

// listPairs lists the remembered pairs, most recently opened first. A missing
// repository or a broken file stays listed, with why.
func (s *Server) listPairs(w http.ResponseWriter, r *http.Request) {
	remembered, err := pair.List(s.root)
	if err != nil {
		writeError(w, err)
		return
	}
	out := []rememberedJSON{}
	for _, rp := range remembered {
		j := rememberedJSON{File: rp.Path, Missing: rp.Missing}
		switch {
		case rp.Err != nil:
			j.Error = rp.Err.Error()
		case filepath.Base(rp.Path) != pair.FileName(rp.Config.Source, rp.Config.Target):
			j.Error = fmt.Sprintf("%s: names the pair %s → %s, whose file is %s",
				rp.Path, rp.Config.Source, rp.Config.Target, pair.FileName(rp.Config.Source, rp.Config.Target))
		default:
			src, tgt := repo(rp.Config.Source), repo(rp.Config.Target)
			j.ID = pair.ID(rp.Config.Source, rp.Config.Target)
			j.Source, j.Target, j.LastOpened = &src, &tgt, rp.Config.LastOpened
		}
		out = append(out, j)
	}
	// The page abbreviates a path under the home directory to "~/…", which it
	// cannot know on its own. An unreadable home is simply not sent.
	home, _ := os.UserHomeDir()
	writeJSON(w, http.StatusOK, struct {
		Home  string           `json:"home,omitempty"`
		Pairs []rememberedJSON `json:"pairs"`
	}{home, out})
}

func (s *Server) getPair(w http.ResponseWriter, r *http.Request) {
	c, err := s.config(r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.pairJSON(c))
}

// openPair opens a pair's repositories and records when it was opened.
func (s *Server) openPair(w http.ResponseWriter, r *http.Request) {
	_, c, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	s.write.Lock()
	c, err = pair.Remember(s.root, c.Source, c.Target, s.now())
	s.write.Unlock()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.pairJSON(c))
}

type settingsRequest struct {
	Exclude []string `json:"exclude"`
	Guard   []string `json:"guard"`
}

// saveSettings replaces a pair's exclusions and content guard. Nothing is saved
// unless every pattern is valid. A preview computed with the settings replaced
// can no longer be applied.
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	s.write.Lock()
	defer s.write.Unlock()
	c, err := s.config(r)
	if err != nil {
		writeError(w, err)
		return
	}
	c.Exclude, c.Guard = req.Exclude, req.Guard
	if _, err := c.Excluded(); err != nil {
		writeError(w, &apiError{Status: http.StatusUnprocessableEntity, Code: "invalid_settings", Message: err.Error()})
		return
	}
	if _, err := c.CompiledGuard(); err != nil {
		writeError(w, &apiError{Status: http.StatusUnprocessableEntity, Code: "invalid_settings", Message: err.Error()})
		return
	}
	if err := c.Save(pair.Path(s.root, c.Source, c.Target)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.pairJSON(c))
}

type patternRequest struct {
	Kind    string `json:"kind"` // "exclude" or "guard"
	Pattern string `json:"pattern"`
	// Literal escapes a guard pattern, so it matches the text as typed.
	Literal bool `json:"literal"`
}

type patternJSON struct {
	Pattern    string `json:"pattern"`              // as it would be saved
	Error      string `json:"error,omitempty"`      // why it would be refused
	Suggestion string `json:"suggestion,omitempty"` // the pattern it most likely meant
}

// checkPattern checks one pattern as it is typed, before it is added. The check
// is floe's own, in Go: neither doublestar globs nor RE2 behave as JavaScript's
// patterns do.
func (s *Server) checkPattern(w http.ResponseWriter, r *http.Request) {
	var req patternRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	out := patternJSON{Pattern: req.Pattern}
	switch {
	case req.Kind == "exclude" && !req.Literal:
		var ee *pair.ExcludeError
		if err := pair.CheckExclude(req.Pattern); errors.As(err, &ee) {
			out.Error, out.Suggestion = ee.Reason, ee.Suggestion
		} else if err != nil {
			out.Error = err.Error()
		}
	case req.Kind == "guard":
		if req.Literal {
			out.Pattern = regexp.QuoteMeta(req.Pattern)
		}
		if _, err := pair.CompileGuard(out.Pattern); err != nil {
			out.Error = err.Error()
		}
	default:
		writeError(w, &apiError{
			Status:  http.StatusBadRequest,
			Code:    "bad_request",
			Message: `kind is "exclude" or "guard", and only a guard pattern can be literal`,
		})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// commits lists the source's first-parent history, newest first.
func (s *Server) commits(w http.ResponseWriter, r *http.Request) {
	p, _, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	branch, err := p.Source.Branch(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	commits, err := p.Source.Commits(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]commitJSON, len(commits))
	for i, c := range commits {
		out[i] = commitJSON{
			ID:          c.ID,
			Parents:     orEmpty(c.Parents),
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
			AuthorTime:  c.AuthorTime,
			Subject:     c.Subject,
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Branch  string       `json:"branch"`
		Commits []commitJSON `json:"commits"`
	}{branch, out})
}

// commit is one source commit's full message and the files it changed, with
// their line counts and exclusions.
func (s *Server) commit(w http.ResponseWriter, r *http.Request) {
	id, err := commitID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, c, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	excluded, err := c.Excluded()
	if err != nil {
		writeError(w, err)
		return
	}
	files, err := p.Files(r.Context(), id, excluded)
	if err != nil {
		writeError(w, err)
		return
	}
	message, err := p.Source.Message(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ID      string     `json:"id"`
		Message string     `json:"message"`
		Files   []fileJSON `json:"files"`
	}{id, message, filesJSON(files)})
}

// diff is a source commit's unified diff of one file, fetched when the file is
// shown.
func (s *Server) diff(w http.ResponseWriter, r *http.Request) {
	id, err := commitID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: "the path to diff is missing"})
		return
	}
	p, _, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := p.Source.FileDiff(r.Context(), id, path)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Path string `json:"path"`
		Diff string `json:"diff"`
	}{path, string(out)})
}

// target is where the target stands: its state, its position against the
// source, and the transfer of floe's waiting in it.
func (s *Server) target(w http.ResponseWriter, r *http.Request) {
	p, c, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	excluded, err := c.Excluded()
	if err != nil {
		writeError(w, err)
		return
	}
	s.write.Lock()
	st, rec, err := s.state(r.Context(), p, c)
	s.write.Unlock()
	if err != nil {
		writeError(w, err)
		return
	}
	pos, err := p.Position(r.Context(), excluded)
	if err != nil {
		writeError(w, err)
		return
	}
	out := targetJSON{
		Branch:    st.Branch,
		Head:      st.Head,
		Dirty:     statusesJSON(st.Dirty),
		Conflicts: orEmpty(st.Conflicts),
		Position:  positionJSON{Match: pos.Match, Nearest: pos.Nearest, Divergent: orEmpty(pos.Divergent)},
	}
	if st.HasMessage {
		out.Message = &st.Message
	}
	if rec != nil {
		out.Transfer = transferJSONOf(rec)
	}
	writeJSON(w, http.StatusOK, out)
}

// state reads the target and the transfer of floe's waiting in it. A recorded
// transfer counts only while the target is as the transfer left it: HEAD where
// it was, tracked changes, and the carried message waiting. Once the user
// commits it, discards it by hand or moves HEAD, the record no longer counts
// and is removed. The caller holds s.write.
func (s *Server) state(ctx context.Context, p *transfer.Pair, c *pair.Config) (*transfer.TargetState, *pair.Transfer, error) {
	st, err := p.State(ctx)
	if err != nil {
		return nil, nil, err
	}
	rec, err := pair.LoadTransfer(s.root, c.Source, c.Target)
	if err != nil || rec == nil {
		return st, nil, err
	}
	if rec.TargetHead == st.Head && len(st.Dirty) > 0 && st.HasMessage {
		return st, rec, nil
	}
	return st, nil, pair.ClearTransfer(s.root, c.Source, c.Target)
}

// conflict reads a conflicted file from the target's working tree, markers and
// all. Only a path git reports as unmerged is read.
func (s *Server) conflict(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	p, _, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	conflicts, err := p.Target.Unmerged(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	notConflicted := &apiError{
		Status:  http.StatusNotFound,
		Code:    "not_conflicted",
		Message: fmt.Sprintf("%s has no conflict markers in %s", path, filepath.Base(p.Target.Dir)),
	}
	if !slices.Contains(conflicts, path) {
		writeError(w, notConflicted)
		return
	}
	full := filepath.Join(p.Target.Dir, filepath.FromSlash(path))
	if fi, err := os.Lstat(full); errors.Is(err, fs.ErrNotExist) {
		writeError(w, notConflicted)
		return
	} else if err != nil {
		writeError(w, err)
		return
	} else if !fi.Mode().IsRegular() {
		writeError(w, &apiError{Status: http.StatusConflict, Code: "not_a_file", Message: path + " is not a regular file"})
		return
	}
	content, err := os.ReadFile(full)
	if err != nil {
		writeError(w, err)
		return
	}
	// Git's own test for a binary file: a NUL in the first 8000 bytes.
	binary := bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0
	if binary {
		content = nil
	}
	writeJSON(w, http.StatusOK, struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Binary  bool   `json:"binary"`
	}{path, string(content), binary})
}

type previewRequest struct {
	Commit string   `json:"commit"`
	Skip   []string `json:"skip"` // paths unticked for this transfer
}

// preview runs every check that can refuse the transfer, and computes what it
// will do. The preview is kept under the id it returns until it is applied or
// the pair's next preview replaces it.
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	if !git.IsObjectID(req.Commit) {
		writeError(w, &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: fmt.Sprintf("%q is not a full commit id", req.Commit)})
		return
	}
	p, c, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	excluded, err := c.Excluded()
	if err != nil {
		writeError(w, err)
		return
	}
	guard, err := c.CompiledGuard()
	if err != nil {
		writeError(w, err)
		return
	}
	id := pair.ID(c.Source, c.Target)

	s.write.Lock()
	defer s.write.Unlock()
	s.mu.Lock()
	delete(s.previews, id)
	s.mu.Unlock()
	pv, err := p.Preview(r.Context(), transfer.Request{Commit: req.Commit, Excluded: excluded, Skip: req.Skip, Guard: guard})
	if err != nil {
		writeError(w, err)
		return
	}
	pend := &pending{id: rand.Text(), preview: pv, skip: req.Skip, exclude: c.Exclude, guard: c.Guard}
	s.mu.Lock()
	s.previews[id] = pend
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, previewJSON{
		ID:         pend.id,
		Commit:     pv.Commit,
		TargetHead: pv.TargetHead,
		Message:    pv.Message,
		Files:      filesJSON(pv.Files),
		Binary:     orEmpty(pv.Binary),
		Result:     resultJSONOf(pv.Result),
	})
}

type applyRequest struct {
	Preview string `json:"preview"` // the id the preview returned
}

// apply carries out the pair's last preview, which the request names. A preview
// the pair's settings have changed since is not applied, and neither is one the
// target's HEAD has moved since: the page previews again. The transfer is
// recorded before it is applied, so that it is known for floe's even if floe
// stops half way.
func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	p, c, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	id := pair.ID(c.Source, c.Target)
	// Once started, an apply runs to the end even if the page goes away.
	ctx := context.WithoutCancel(r.Context())

	s.write.Lock()
	defer s.write.Unlock()
	s.mu.Lock()
	pend := s.previews[id]
	if pend != nil && pend.id == req.Preview {
		delete(s.previews, id)
	} else {
		pend = nil
	}
	s.mu.Unlock()
	switch {
	case pend == nil:
		writeError(w, &apiError{Status: http.StatusConflict, Code: "preview_gone", Message: "this preview is no longer current: preview the transfer again"})
		return
	case !slices.Equal(pend.exclude, c.Exclude) || !slices.Equal(pend.guard, c.Guard):
		writeError(w, &apiError{Status: http.StatusConflict, Code: "preview_gone", Message: "the pair's settings changed since this preview: preview the transfer again"})
		return
	}

	rec := &pair.Transfer{
		Commit:     pend.preview.Commit,
		TargetHead: pend.preview.TargetHead,
		Skip:       pend.skip,
		Applied:    s.now(),
	}
	if err := pair.SaveTransfer(s.root, c.Source, c.Target, rec); err != nil {
		writeError(w, err)
		return
	}
	res, err := p.Apply(ctx, pend.preview)
	if err != nil {
		// A refused or failed apply has written nothing, or at least no carried
		// message, so the record could not count anyway.
		writeError(w, errors.Join(err, pair.ClearTransfer(s.root, c.Source, c.Target)))
		return
	}
	writeJSON(w, http.StatusOK, appliedJSON{Transfer: transferJSONOf(rec), Result: resultJSONOf(res)})
}

// discard throws away the transfer of floe's waiting in the target, conflict
// resolutions and all. Changes floe did not make are never discarded: without a
// recorded transfer that still counts, it refuses.
func (s *Server) discard(w http.ResponseWriter, r *http.Request) {
	p, c, err := s.open(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := context.WithoutCancel(r.Context())

	s.write.Lock()
	defer s.write.Unlock()
	_, rec, err := s.state(ctx, p, c)
	if err != nil {
		writeError(w, err)
		return
	}
	if rec == nil {
		writeError(w, &apiError{
			Status:  http.StatusConflict,
			Code:    "no_transfer",
			Message: fmt.Sprintf("there is no transfer of floe's waiting in %s to discard", filepath.Base(c.Target)),
		})
		return
	}
	if err := p.Abort(ctx); err != nil {
		writeError(w, err)
		return
	}
	if err := pair.ClearTransfer(s.root, c.Source, c.Target); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}
