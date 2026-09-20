// Package server is floe's HTTP side: the embedded page, and the JSON API the
// page drives a transfer through. It answers only on the loopback interface,
// only requests addressed to it by that address, and only API requests that
// carry the launch's token.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Sknoww/floe/internal/transfer"
)

// TokenHeader carries the launch's token on every API request. The page reads
// the token from its URL's fragment, which the browser never sends to a server.
const TokenHeader = "X-Floe-Token"

// Options configure a Server.
type Options struct {
	Root   string   // floe's config directory, as pair.Root finds it
	Addr   net.Addr // the listener's address: requests must be addressed to it
	Assets fs.FS    // the built frontend
}

// Server serves one launch of floe.
type Server struct {
	root    string
	token   string
	host    string   // "127.0.0.1:<port>"
	hosts   []string // the Host values answered
	origins []string // the Origin values answered
	assets  fs.FS
	now     func() time.Time

	// write is held by everything that writes to a target or to a pair's
	// files, and by every read of a recorded transfer, so a state read can
	// never clear the record of a transfer while it is being applied.
	write sync.Mutex

	mu       sync.Mutex
	opened   map[string]*transfer.Pair // pairs whose repositories are open, by ID
	previews map[string]*pending       // each pair's last preview, by pair ID
}

// pending is a preview waiting to be applied.
type pending struct {
	id      string
	preview *transfer.Preview
	skip    []string
	// exclude and guard are the pair's settings the preview was computed with.
	exclude, guard []string
}

// New makes a server for a listener on the loopback interface.
func New(opts Options) (*Server, error) {
	addr, ok := opts.Addr.(*net.TCPAddr)
	if !ok || !addr.IP.IsLoopback() {
		return nil, fmt.Errorf("floe serves only on the loopback interface, not on %v", opts.Addr)
	}
	s := &Server{
		root:     opts.Root,
		token:    rand.Text(),
		host:     addr.String(),
		assets:   opts.Assets,
		now:      time.Now,
		opened:   map[string]*transfer.Pair{},
		previews: map[string]*pending{},
	}
	s.hosts = []string{s.host}
	s.origins = []string{"http://" + s.host}
	if devOrigin != "" {
		u, err := url.Parse(devOrigin)
		if err != nil {
			return nil, err
		}
		s.hosts = append(s.hosts, u.Host)
		s.origins = append(s.origins, devOrigin)
	}
	return s, nil
}

// URL is the page's address for this launch, with the token — and the pair to
// open, when there is one — in its fragment. A dev build points at the Vite dev
// server instead.
func (s *Server) URL(pairID string) string {
	fragment := url.Values{"token": {s.token}}
	if pairID != "" {
		fragment.Set("pair", pairID)
	}
	base := "http://" + s.host
	if devOrigin != "" {
		base = devOrigin
	}
	return base + "/#" + fragment.Encode()
}

// Serve answers requests on ln until ctx is done, then lets the requests in
// flight finish.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// Handler routes the page and the API, behind their checks.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/pairs", s.listPairs)
	api.HandleFunc("GET /api/pairs/{pair}", s.getPair)
	api.HandleFunc("POST /api/pairs/{pair}/open", s.openPair)
	api.HandleFunc("PUT /api/pairs/{pair}/settings", s.saveSettings)
	api.HandleFunc("POST /api/patterns/check", s.checkPattern)
	api.HandleFunc("GET /api/pairs/{pair}/commits", s.commits)
	api.HandleFunc("GET /api/pairs/{pair}/commits/{commit}", s.commit)
	api.HandleFunc("GET /api/pairs/{pair}/commits/{commit}/diff", s.diff)
	api.HandleFunc("GET /api/pairs/{pair}/target", s.target)
	api.HandleFunc("GET /api/pairs/{pair}/conflict", s.conflict)
	api.HandleFunc("POST /api/pairs/{pair}/preview", s.preview)
	api.HandleFunc("POST /api/pairs/{pair}/apply", s.apply)
	api.HandleFunc("POST /api/pairs/{pair}/discard", s.discard)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, &apiError{
			Status:  http.StatusNotFound,
			Code:    "not_found",
			Message: r.Method + " " + r.URL.Path + " is not part of floe's API",
		})
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", s.checkToken(api))
	mux.HandleFunc("/", s.page)
	return s.checkHost(mux)
}

// checkHost answers only requests addressed to this server by its own address,
// so a page on a name rebound to 127.0.0.1 is refused. Every response is kept
// out of other pages' frames.
func (s *Server) checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(s.hosts, r.Host) {
			http.Error(w, "floe answers only requests addressed to "+s.host, http.StatusMisdirectedRequest)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// checkToken refuses an API request from another origin, or without this
// launch's token. floe sends no CORS headers, so another origin's page cannot
// send the token header at all.
func (s *Server) checkToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if origin := r.Header.Get("Origin"); origin != "" && !slices.Contains(s.origins, origin) {
			writeError(w, &apiError{Status: http.StatusForbidden, Code: "origin", Message: "floe refuses requests from " + origin})
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(s.token)) != 1 {
			writeError(w, &apiError{
				Status:  http.StatusUnauthorized,
				Code:    "token",
				Message: "the request lacks this launch's token: open floe at the address it printed",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// page serves the built frontend. Its files carry no secrets, so no token is
// asked for.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if fi, err := fs.Stat(s.assets, name); err != nil || fi.IsDir() {
		if name == "index.html" {
			http.Error(w, "This floe was built without its frontend: run npm run build in web/, then build floe again.", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, s.assets, name)
}
