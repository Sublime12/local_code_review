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

func main() {
	portFlag := flag.Int("port", 0, "Port to listen on (0 for automatic free port)")
	noBrowserFlag := flag.Bool("no-browser", false, "Do not open browser automatically")
	refFlag := flag.String("ref", "", "Git ref to compare against (e.g. main, HEAD~1)")
	flag.Parse()

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

	_ = refFlag

	addr := fmt.Sprintf("127.0.0.1:%d", *portFlag)
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

	srv := server.NewServer(repoRoot, webAssets)
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
	fmt.Printf(" Server running at: %s\n", url)
	fmt.Println(" Press Ctrl+C to stop the review server")
	fmt.Println("=======================================================")

	if !*noBrowserFlag {
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
