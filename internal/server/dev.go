//go:build dev

package server

// devOrigin is the Vite dev server, which a dev build also answers and opens
// the page on: it serves the frontend with hot reload and proxies /api to floe.
// A release build never answers it.
const devOrigin = "http://localhost:5173"

// ListenAddr is where floe listens: in a dev build, a fixed port for the dev
// server's proxy to find.
const ListenAddr = "127.0.0.1:5174"
