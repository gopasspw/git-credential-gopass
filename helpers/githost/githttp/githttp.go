package githttp

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitPath is the path to the git binary
var GitPath = "git"

// Middleware for Basic Authentication
func BasicAuthMiddleware(next http.HandlerFunc, username, password string) http.HandlerFunc {
	// If no auth credentials provided via flags, skip auth
	useAuth := (username != "" && password != "")
	if !useAuth {
		log.Println("Warning: No authentication configured (-auth-user and -auth-pass not set)")
		return next
	}
	log.Println("Basic Authentication enabled")

	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Printf("Auth Required for %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			w.Header().Set("WWW-Authenticate", `Basic realm="Git Repository"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Expecting "Basic <base64data>"
		const prefix = "Basic "
		if !strings.HasPrefix(authHeader, prefix) {
			log.Printf("Invalid Authorization header format from %s", r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		encoded := authHeader[len(prefix):]
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			log.Printf("Error decoding base64 auth data from %s: %v", r.RemoteAddr, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Expecting "username:password"
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			log.Printf("Invalid decoded auth data format from %s", r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		providedUser := parts[0]
		providedPass := parts[1]

		if providedUser != username || providedPass != password {
			log.Printf("Authentication failed for user '%s' from %s", providedUser, r.RemoteAddr)
			w.Header().Set("WWW-Authenticate", `Basic realm="Git Repository"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Authentication successful, pass the username down via context or header if needed
		// For git-http-backend, we'll set REMOTE_USER environment variable later.
		log.Printf("User '%s' authenticated successfully from %s", providedUser, r.RemoteAddr)

		// Add the username to the request context
		rc := r.WithContext(WithUsername(r.Context(), providedUser))

		// Pass control to the next handler
		next(w, rc)
	}
}

// Core Git handler (now receives authenticated user if auth enabled)
func GitHandler(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authenticatedUser := Username(r.Context())

		log.Printf("Handling %s %s for user '%s' from %s", r.Method, r.URL.String(), authenticatedUser, r.RemoteAddr)

		// Basic path validation (prevent escaping root)
		requestPath := filepath.Clean(r.URL.Path)
		if strings.HasPrefix(requestPath, "..") || !strings.HasPrefix(requestPath, "/") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		// Prepare command execution
		cmd := exec.Command(GitPath, "http-backend")

		// Set required environment variables for git-http-backend
		remoteAddr, _, _ := net.SplitHostPort(r.RemoteAddr)
		env := []string{
			fmt.Sprintf("GIT_PROJECT_ROOT=%s", root),
			"GIT_HTTP_EXPORT_ALL=",
			fmt.Sprintf("PATH_INFO=%s", requestPath),
			fmt.Sprintf("REMOTE_ADDR=%s", remoteAddr),
			fmt.Sprintf("REQUEST_METHOD=%s", r.Method),
			fmt.Sprintf("QUERY_STRING=%s", r.URL.RawQuery),
			fmt.Sprintf("CONTENT_TYPE=%s", r.Header.Get("Content-Type")),
		}
		// Pass authenticated user if available
		if authenticatedUser != "" {
			env = append(env, fmt.Sprintf("REMOTE_USER=%s", authenticatedUser))
		}
		cmd.Env = append(os.Environ(), env...)

		// Pipe request body to command stdin for POST requests
		if r.Method == "POST" && r.Body != nil {
			cmd.Stdin = r.Body
			defer r.Body.Close()
		} else {
			cmd.Stdin = nil
		}

		// Capture stdout and stderr
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("Error creating stdout pipe: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			log.Printf("Error creating stderr pipe: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			log.Printf("Error starting git http-backend: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Log stderr in the background
		go func() {
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				log.Printf("git-http-backend stderr: %s", scanner.Text())
			}
		}()

		// Process CGI headers from stdout
		stdoutReader := bufio.NewReader(stdoutPipe)
		statusCode := http.StatusOK // Default status
		for {
			line, err := stdoutReader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading CGI headers: %v", err)
				}
				break
			}

			line = strings.TrimSpace(line)
			if line == "" {
				// Empty line marks the end of headers
				break
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				headerName := strings.TrimSpace(parts[0])
				headerValue := strings.TrimSpace(parts[1])

				if strings.EqualFold(headerName, "Status") {
					fmt.Sscanf(headerValue, "%d", &statusCode)
				} else {
					w.Header().Set(headerName, headerValue)
				}
			} else {
				log.Printf("Ignoring malformed CGI header line: %s", line)
			}
		}

		// Write status code and copy remaining stdout (body) to response
		w.WriteHeader(statusCode)
		_, err = io.Copy(w, stdoutReader)
		if err != nil {
			log.Printf("Error copying git-http-backend output to response: %v", err)
		}

		// Wait for the command to finish
		if err := cmd.Wait(); err != nil {
			log.Printf("git http-backend command finished with error: %v", err)
		}
	}
}
