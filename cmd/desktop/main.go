// Command desktop is the macOS window around yt-studio.
//
// It owns no application logic. It starts the server binary sitting beside it,
// waits for the port to answer, and points a WKWebView at it — so the thing in
// the window is the same SPA the browser gets, talking to the same API over the
// same HTTP, with the same event stream.
//
// Two processes rather than one, deliberately. A native window needs cgo; the
// server is built CGO_ENABLED=0 and its pure-Go SQLite is the reason. Keeping
// the window in its own binary leaves the server exactly as it is built and
// tested everywhere else, and costs one exec.
//
// The webview is not given the embedded assets to serve. A WKWebView custom
// scheme handler buffers its responses, which would break /events: the UI is
// kept current by a long-lived SSE stream, and a buffered stream is no stream.
// A real listener on a real port has none of that problem.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	webview "github.com/webview/webview_go"
)

// serverBinary is what the server is called inside the bundle, beside this one.
const serverBinary = "yt-studio"

// startupTimeout bounds the wait for the server to answer. Generous: the first
// run of a new installation applies every migration before it listens.
const startupTimeout = 60 * time.Second

// pollInterval is how often the port is tried while waiting.
const pollInterval = 100 * time.Millisecond

// shutdownGrace is how long the server has to flush and exit after being asked,
// before it is killed outright.
const shutdownGrace = 10 * time.Second

type cli struct {
	//nolint:lll // one flag, one line
	URL string `help:"Open this address instead of starting a server. This is what make dev-desktop uses, pointing the window at the Vite dev server." `
	//nolint:lll // one flag, one line
	Home string `help:"Passed through to the server: the directory holding the database, assets, resources and transcripts." env:"YTS_HOME"`
	//nolint:lll // one flag, one line
	Server string `help:"Path to the server binary. Defaults to the one beside this executable."`
	//nolint:lll // one flag, one line
	Width int `help:"Initial window width." default:"1440"`
	//nolint:lll // one flag, one line
	Height int `help:"Initial window height." default:"900"`
}

func main() {
	var root cli
	kong.Parse(&root,
		kong.Name("yt-studio-desktop"),
		kong.Description("The macOS window around yt-studio."),
		kong.UsageOnError(),
	)
	if err := root.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "yt-studio-desktop:", err)
		os.Exit(1)
	}
}

func (c *cli) Run() error {
	url := c.URL
	if url == "" {
		// Own the server: start it, and stop it when the window closes. A
		// server left running after its only window is gone would hold the
		// database against the next launch.
		server, stop, err := c.startServer()
		if err != nil {
			return err
		}
		defer stop()
		url = server
	}

	if err := waitForServer(url); err != nil {
		return err
	}

	// webview must own the main goroutine: AppKit will not accept UI calls from
	// anywhere else, and Go's runtime locks main to the initial thread.
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("yt-studio")
	w.SetSize(c.Width, c.Height, webview.HintNone)

	// A signal would otherwise end the process where it stands, and Go runs no
	// deferred function on the way out — which would leave the server running
	// with nothing to close it, holding the database against the next launch.
	// Terminate breaks the UI loop instead, so Run returns and the defers above
	// happen exactly as they do when the window is closed by hand.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		if _, ok := <-signals; ok {
			w.Terminate()
		}
	}()

	w.Navigate(url)
	w.Run()
	return nil
}

// startServer launches the server on a free port and returns its address along
// with the function that stops it.
func (c *cli) startServer() (string, func(), error) {
	binary, err := c.serverPath()
	if err != nil {
		return "", nil, err
	}
	addr, err := freeAddress()
	if err != nil {
		return "", nil, err
	}

	args := []string{"serve", "--listen", addr}
	if c.Home != "" {
		args = append(args, "--home", c.Home)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // argv is built here, never from input
	// Interrupt rather than the default kill: the server flushes the scheduler's
	// final transitions on the way down, and losing those would leave tasks
	// claiming to be running after the window is gone. WaitDelay is the promise
	// that it still dies if it ignores that.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = shutdownGrace
	// Inherited, so a terminal launch still shows the log; a Finder launch has
	// nowhere to show it, which is why the server also writes one to its home.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return "", nil, fmt.Errorf("start %s: %w", binary, err)
	}

	stop := func() {
		cancel()
		_ = cmd.Wait()
	}
	return "http://" + addr, stop, nil
}

// serverPath finds the server binary beside this executable, which is how the
// bundle lays the two out.
func (c *cli) serverPath() (string, error) {
	if c.Server != "" {
		return c.Server, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate this executable: %w", err)
	}
	// Symlinks resolved, so a binary reached through one still finds its
	// sibling rather than looking beside the link.
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	candidate := filepath.Join(filepath.Dir(self), serverBinary)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("no server binary at %s: %w", candidate, err)
	}
	return candidate, nil
}

// freeAddress reserves a port by binding it and letting go. The gap between
// closing and the server binding is a race in principle; on a single-operator
// machine it has no other runner to lose to, and the alternative is parsing the
// address back out of the server's log.
func freeAddress() (string, error) {
	var config net.ListenConfig
	listener, err := config.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("reserve a port: %w", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", fmt.Errorf("release the reserved port: %w", err)
	}
	return addr, nil
}

// waitForServer blocks until the health endpoint answers. Without it the window
// opens on a connection error and stays there: the webview does not retry, and
// the operator sees a failure that was only ever a race with startup.
func waitForServer(base string) error {
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/health", http.NoBody)
		if err != nil {
			return fmt.Errorf("build health request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s did not answer within %s: %w", base, startupTimeout, errors.Join(err, ctx.Err()))
		case <-ticker.C:
		}
	}
}
