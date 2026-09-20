// Command floe carries commits from one git repository into another that shares
// none of its history. `floe <source> <target>` opens the pair in the browser,
// and bare `floe` lists the pairs it remembers. It never commits and never
// pushes.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Sknoww/floe/internal/git"
	"github.com/Sknoww/floe/internal/pair"
	"github.com/Sknoww/floe/internal/server"
	"github.com/Sknoww/floe/internal/transfer"
	"github.com/Sknoww/floe/web"
)

// version is stamped by the release build.
var version = "dev"

const usage = `usage: floe [<source> <target>]

  floe <source> <target>   open a pair of repositories, and remember it
  floe                     list the pairs floe remembers`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "floe:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 {
		switch args[0] {
		case "-h", "--help", "help":
			fmt.Println(usage)
			return nil
		case "-v", "--version", "version":
			fmt.Println("floe", version)
			return nil
		}
	}
	if len(args) != 0 && len(args) != 2 {
		return errors.New(usage)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := git.CheckVersion(ctx); err != nil {
		return err
	}
	root, err := pair.Root()
	if err != nil {
		return err
	}
	var id string
	if len(args) == 2 {
		// A pair floe cannot transfer between is refused here, in the
		// terminal, before a browser opens on nothing.
		p, err := transfer.Open(ctx, args[0], args[1])
		if err != nil {
			return err
		}
		if _, err := pair.Remember(root, p.Source.Dir, p.Target.Dir, time.Now()); err != nil {
			return err
		}
		id = pair.ID(p.Source.Dir, p.Target.Dir)
	}

	ln, err := net.Listen("tcp", server.ListenAddr)
	if err != nil {
		return err
	}
	s, err := server.New(server.Options{Root: root, Addr: ln.Addr(), Assets: web.Dist()})
	if err != nil {
		ln.Close()
		return err
	}
	url := s.URL(id)
	fmt.Printf("floe is running at %s\nPress Ctrl-C to stop.\n", url)
	if err := openBrowser(url); err != nil {
		fmt.Fprintln(os.Stderr, "floe: open the address above in a browser:", err)
	}
	return s.Serve(ctx, ln)
}

// openBrowser opens url in the default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("floe does not know how to open a browser on %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // xdg-open can stay until the browser exits
	return nil
}
