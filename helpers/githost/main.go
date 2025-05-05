package main

import (
	"flag"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"

	"github.com/gopasspw/git-credential-gopass/helpers/githost/githttp"
)

var (
	repoRoot   = flag.String("repo-root", "", "Path to the directory containing bare Git repositories")
	listenAddr = flag.String("listen-addr", "localhost:8080", "Address and port to listen on")
	gitBinPath = flag.String("git-bin-path", "git", "Path to the git binary")
	authUser   = flag.String("auth-user", "gopass", "Username for Basic Authentication (required if auth-pass is set)")
	authPass   = flag.String("auth-pass", "pass", "Password for Basic Authentication (required if auth-user is set)")
)

func main() {
	flag.Parse()

	if *repoRoot == "" {
		log.Fatal("Error: -repo-root flag is required")
	}
	// Validate auth flags together
	if (*authUser != "" && *authPass == "") || (*authUser == "" && *authPass != "") {
		log.Fatal("Error: Both -auth-user and -auth-pass must be provided together, or neither.")
	}

	absRepoRoot, err := filepath.Abs(*repoRoot)
	if err != nil {
		log.Fatalf("Error getting absolute path for repo-root: %v", err)
	}
	log.Printf("Serving Git repositories from: %s", absRepoRoot)

	// Check if git binary exists
	_, err = exec.LookPath(*gitBinPath)
	if err != nil {
		log.Fatalf("Error: git binary not found at '%s' or in PATH: %v", *gitBinPath, err)
	}
	githttp.GitPath = *gitBinPath

	// Wrap the gitHandler with the auth middleware
	finalHandler := githttp.BasicAuthMiddleware(githttp.GitHandler(absRepoRoot), *authUser, *authPass)
	http.HandleFunc("/", finalHandler)

	log.Printf("Starting Git HTTP server on %s", *listenAddr)
	err = http.ListenAndServe(*listenAddr, nil)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
