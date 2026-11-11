package main

import (
	"embed"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Sublime12/local_code_review/git"
	"github.com/Sublime12/local_code_review/server"
)

//go:embed all:web
var webAssets embed.FS

type cliOptions struct {
	port    int
	browser bool
	ref     string
}

func parseFlags(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	var opts cliOptions
	fs.IntVar(&opts.port, "port", 0, "Port to listen on (0 for automatic free port)")
	fs.BoolVar(&opts.browser, "browser", false, "Open browser automatically")
	fs.StringVar(&opts.ref, "ref", "", "Git ref to compare against (e.g. main, HEAD~1)")
	err := fs.Parse(args)
	return opts, err
}

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}

	isGit, err := git.IsInsideWorkTree()
	if err != nil || !isGit {
		fmt.Fprintf(os.Stderr, "Error: Current directory is not inside a Git repository.\n")
		os.Exit(1)
	}

	repoRoot, err := git.GetRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error determining git repository root: %v\n", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", opts.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start listener: %v\n", err)
			os.Exit(1)
		}
	}
	defer listener.Close()

	assignedPort := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://localhost:%d", assignedPort)

	srv := server.NewServer(repoRoot, webAssets, opts.ref)
	httpServer := &http.Server{
		Handler: srv.Handler(),
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	fmt.Println("=======================================================")
	fmt.Println(" Local Code Review")
	fmt.Printf(" Repository: %s\n", repoRoot)
	if opts.ref != "" {
		fmt.Printf(" Comparing against ref: %s\n", opts.ref)
	}
	fmt.Printf(" Server running at: %s\n", url)
	fmt.Println(" Press Ctrl+C to stop the review server")
	fmt.Println("=======================================================")

	if opts.browser {
		go func() {
			time.Sleep(100 * time.Millisecond)
			openBrowser(url)
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nStopping review server. Goodbye!")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
