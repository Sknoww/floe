package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Sknoww/floe/internal/git"
	"github.com/Sknoww/floe/internal/pair"
	"github.com/Sknoww/floe/internal/transfer"
)

// apiError is a refusal or a failure as the page receives it:
// {"error": {"code": …, "message": …}}. Code is what the page acts on; Message
// is floe's or git's own words, to be shown as they are.
type apiError struct {
	Status  int         `json:"-"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Paths   []string    `json:"paths,omitempty"`   // "dirty": the target's changed files
	Matches []matchJSON `json:"matches,omitempty"` // "guard": every match
}

func (e *apiError) Error() string { return e.Message }

// asAPIError classifies an error from the layers below. What floe cannot
// classify is a failure, reported in its own words.
func asAPIError(err error) *apiError {
	var (
		ae    *apiError
		dirty *transfer.DirtyError
		guard *transfer.GuardError
		apply *git.ApplyError
	)
	conflict := func(code string) *apiError {
		return &apiError{Status: http.StatusConflict, Code: code, Message: err.Error()}
	}
	switch {
	case errors.As(err, &ae):
		return ae
	case errors.As(err, &dirty):
		e := conflict("dirty")
		e.Paths = dirty.Paths
		return e
	case errors.As(err, &guard):
		e := conflict("guard")
		e.Matches = matchesJSON(guard.Matches)
		return e
	case errors.As(err, &apply):
		e := conflict("apply_refused")
		e.Message = apply.Stderr
		return e
	case errors.Is(err, transfer.ErrStale):
		return conflict("stale")
	case errors.Is(err, transfer.ErrEmptyCommit):
		return conflict("empty_commit")
	case errors.Is(err, transfer.ErrNothingToTransfer):
		return conflict("nothing_to_transfer")
	}
	return &apiError{Status: http.StatusInternalServerError, Code: "failed", Message: err.Error()}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v) // the status is already sent; a failed write has no one to tell
}

func writeError(w http.ResponseWriter, err error) {
	e := asAPIError(err)
	writeJSON(w, e.Status, struct {
		Error *apiError `json:"error"`
	}{e})
}

// readJSON decodes a request body strictly: one object of at most a megabyte,
// with no unknown fields.
func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil {
		if _, next := dec.Token(); next != io.EOF {
			err = errors.New("unexpected data after the JSON object")
		}
	}
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: "the request body: " + err.Error()}
	}
	return nil
}

// orEmpty makes a nil slice encode as [] rather than null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

type repoJSON struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func repo(path string) repoJSON { return repoJSON{Name: filepath.Base(path), Path: path} }

type pairJSON struct {
	ID         string    `json:"id"`
	File       string    `json:"file"`
	Source     repoJSON  `json:"source"`
	Target     repoJSON  `json:"target"`
	Exclude    []string  `json:"exclude"`
	Guard      []string  `json:"guard"`
	LastOpened time.Time `json:"lastOpened,omitzero"`
}

func (s *Server) pairJSON(c *pair.Config) pairJSON {
	return pairJSON{
		ID:         pair.ID(c.Source, c.Target),
		File:       pair.Path(s.root, c.Source, c.Target),
		Source:     repo(c.Source),
		Target:     repo(c.Target),
		Exclude:    orEmpty(c.Exclude),
		Guard:      orEmpty(c.Guard),
		LastOpened: c.LastOpened,
	}
}

// rememberedJSON is one pair file. A file floe cannot read, or that names
// another pair, has only its path and the error.
type rememberedJSON struct {
	ID         string    `json:"id,omitempty"`
	File       string    `json:"file"`
	Source     *repoJSON `json:"source,omitempty"`
	Target     *repoJSON `json:"target,omitempty"`
	LastOpened time.Time `json:"lastOpened,omitzero"`
	Missing    []string  `json:"missing,omitempty"` // repository paths no longer there
	Error      string    `json:"error,omitempty"`
}

type commitJSON struct {
	ID          string    `json:"id"`
	Parents     []string  `json:"parents"`
	AuthorName  string    `json:"authorName"`
	AuthorEmail string    `json:"authorEmail"`
	AuthorTime  time.Time `json:"authorTime"`
	Subject     string    `json:"subject"`
}

type fileJSON struct {
	Path     string `json:"path"`
	Status   string `json:"status"`
	Added    int    `json:"added"`
	Deleted  int    `json:"deleted"`
	Binary   bool   `json:"binary"`
	Excluded bool   `json:"excluded"`
	Skipped  bool   `json:"skipped"`
}

func filesJSON(files []transfer.File) []fileJSON {
	out := make([]fileJSON, len(files))
	for i, f := range files {
		out[i] = fileJSON{
			Path:     f.Path,
			Status:   string(f.Status),
			Added:    f.Lines.Added,
			Deleted:  f.Lines.Deleted,
			Binary:   f.Lines.Binary,
			Excluded: f.Excluded,
			Skipped:  f.Skipped,
		}
	}
	return out
}

// changeJSON is a path and its status letter.
type changeJSON struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func statusesJSON(files []git.FileStatus) []changeJSON {
	out := make([]changeJSON, len(files))
	for i, f := range files {
		out[i] = changeJSON{Path: f.Path, Status: string(f.Status)}
	}
	return out
}

type resultJSON struct {
	Files     []changeJSON `json:"files"` // a conflicted file has status "U"
	Conflicts []string     `json:"conflicts"`
	// Conflicted is each conflicted text file as the apply writes it; a
	// preview's only.
	Conflicted map[string]string `json:"conflicted,omitempty"`
}

func resultJSONOf(res *git.ApplyResult) resultJSON {
	out := resultJSON{
		Files:      make([]changeJSON, len(res.Files)),
		Conflicts:  orEmpty(res.Conflicts),
		Conflicted: res.Conflicted,
	}
	for i, c := range res.Files {
		out.Files[i] = changeJSON{Path: c.Path, Status: string(c.Status)}
	}
	return out
}

type matchJSON struct {
	Pattern string     `json:"pattern"`
	Path    string     `json:"path"` // "" for the commit message
	LineNo  int        `json:"lineNo"`
	Line    string     `json:"line"`
	Parts   []partJSON `json:"parts"` // Line, cut where the pattern matched
	Merged  bool       `json:"merged"`
}

type partJSON struct {
	Text    string `json:"text"`
	Matched bool   `json:"matched,omitempty"`
}

func matchesJSON(matches []transfer.GuardMatch) []matchJSON {
	out := make([]matchJSON, len(matches))
	for i, m := range matches {
		out[i] = matchJSON{
			Pattern: m.Pattern,
			Path:    m.Path,
			LineNo:  m.LineNo,
			Line:    m.Line,
			Parts:   parts(m.Line, m.Spans),
			Merged:  m.Merged,
		}
	}
	return out
}

// parts cuts a matched line at the byte spans the pattern matched, so the page
// can mark them without counting bytes: JavaScript indexes a string by UTF-16
// unit, not by byte.
func parts(line string, spans [][2]int) []partJSON {
	out := []partJSON{}
	at := 0
	for _, sp := range spans {
		if sp[0] > at {
			out = append(out, partJSON{Text: line[at:sp[0]]})
		}
		out = append(out, partJSON{Text: line[sp[0]:sp[1]], Matched: true})
		at = sp[1]
	}
	if at < len(line) {
		out = append(out, partJSON{Text: line[at:]})
	}
	return out
}

type positionJSON struct {
	Match     string   `json:"match"`   // "" when no commit matches
	Nearest   string   `json:"nearest"` // set only when nothing matches
	Divergent []string `json:"divergent"`
}

type transferJSON struct {
	Commit     string    `json:"commit"`
	TargetHead string    `json:"targetHead"`
	Skip       []string  `json:"skip"`
	Applied    time.Time `json:"applied"`
}

func transferJSONOf(t *pair.Transfer) *transferJSON {
	return &transferJSON{Commit: t.Commit, TargetHead: t.TargetHead, Skip: orEmpty(t.Skip), Applied: t.Applied}
}

type targetJSON struct {
	Branch    string       `json:"branch"` // "" when HEAD is detached
	Head      string       `json:"head"`   // "" for a target with no commits
	Dirty     []changeJSON `json:"dirty"`
	Conflicts []string     `json:"conflicts"`
	// Message is the carried message waiting in SQUASH_MSG, or null.
	Message  *string      `json:"message"`
	Position positionJSON `json:"position"`
	// Transfer is the transfer of floe's waiting in the target, or null:
	// changes without one are the user's, and are never discarded.
	Transfer *transferJSON `json:"transfer"`
}

type previewJSON struct {
	ID         string     `json:"id"` // what an apply names
	Commit     string     `json:"commit"`
	TargetHead string     `json:"targetHead"`
	Message    string     `json:"message"`
	Files      []fileJSON `json:"files"`
	Binary     []string   `json:"binary"` // crossing files the guard cannot scan
	Result     resultJSON `json:"result"`
}

type appliedJSON struct {
	Transfer *transferJSON `json:"transfer"`
	Result   resultJSON    `json:"result"`
}
