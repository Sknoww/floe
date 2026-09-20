//go:build !dev

package server

// devOrigin is empty: a release build answers only itself.
const devOrigin = ""

// ListenAddr is where floe listens: the loopback interface, on a port the
// system picks.
const ListenAddr = "127.0.0.1:0"
