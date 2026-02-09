package main

import (
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/gopasspw/git-credential-gopass/helpers/githost/githttp"
	"github.com/gopasspw/gopass/helpers/gitutils"
	"github.com/gopasspw/gopass/pkg/ctxutil"
	"github.com/gopasspw/gopass/pkg/fsutil"
	"github.com/gopasspw/gopass/pkg/gopass/apimock"
	"github.com/gopasspw/gopass/pkg/termio"
	"github.com/gopasspw/gopass/tests/gptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestGitCredentialFormat(t *testing.T) {
	t.Parallel()

	data := []io.Reader{
		strings.NewReader("" +
			"protocol=https\n" +
			"host=example.com\n" +
			"username=bob\n" +
			"foo=bar\n" +
			"path=test\n" +
			"password=secr3=t\n",
		),
		strings.NewReader("" +
			"protocol=https\n" +
			"host=example.com\n" +
			"username=bob\n" +
			"foo=bar\n" +
			"path=test\n" +
			"password=secr3=t\n" +
			"password_expiry_utc=2000\n" +
			"oauth_refresh_token=xyzzy\n",
		),
		strings.NewReader("" +
			"protocol=https\n" +
			"host=example.com\n" +
			"username=bob\n" +
			"foo=bar\n" +
			"password=secr3=t\n" +
			"test=",
		),
		strings.NewReader("" +
			"protocol=https\n" +
			"host=example.com\n" +
			"username=bob\n" +
			"foo=bar\n" +
			"password=secr3=t\n" +
			"test",
		),
	}

	results := []gitCredentials{
		{
			Host:     "example.com",
			Password: "secr3=t",
			Path:     "test",
			Protocol: "https",
			Username: "bob",
		},
		{
			Host:              "example.com",
			Password:          "secr3=t",
			Path:              "test",
			Protocol:          "https",
			Username:          "bob",
			PasswordExpiryUTC: "2000",
			OAuthRefreshToken: "xyzzy",
		},
		{},
		{},
	}

	expectsErr := []bool{false, false, true, true}
	for i := range data {
		result, err := parseGitCredentials(data[i])
		if expectsErr[i] {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		if err != nil {
			continue
		}
		assert.Equal(t, results[i], *result)
		buf := &bytes.Buffer{}
		n, err := result.WriteTo(buf)
		require.NoError(t, err, "could not serialize credentials")
		assert.Equal(t, buf.Len(), int(n))
		parseback, err := parseGitCredentials(buf)
		require.NoError(t, err, "failed parsing my own output")
		assert.Equal(t, results[i], *parseback, "failed parsing my own output")
	}
}

func TestGitCredentialHelper(t *testing.T) { //nolint:paralleltest
	ctx := t.Context()
	act := &gc{
		gp: apimock.New(),
	}
	require.NoError(t, act.gp.Set(ctx, "foo", &apimock.Secret{Buf: []byte("bar")}))

	stdout := &bytes.Buffer{}
	Stdout = stdout
	color.NoColor = true
	defer func() {
		Stdout = os.Stdout
		termio.Stdin = os.Stdin
	}()

	c := gptest.CliCtx(ctx, t)

	// before without stdin
	require.Error(t, act.Before(c))

	// before with stdin
	ctx = ctxutil.WithStdin(ctx, true)
	c.Context = ctx
	require.NoError(t, act.Before(c))

	s := "protocol=https\n" +
		"host=example.com\n" +
		"username=bob\n"

	termio.Stdin = strings.NewReader(s)
	require.NoError(t, act.Get(c))
	assert.Empty(t, stdout.String())

	termio.Stdin = strings.NewReader(s + "password=secr3=t\n")
	require.NoError(t, act.Store(c))
	stdout.Reset()

	termio.Stdin = strings.NewReader(s)
	require.NoError(t, act.Get(c))
	read, err := parseGitCredentials(stdout)
	require.NoError(t, err)
	assert.Equal(t, "secr3=t", read.Password)
	stdout.Reset()

	termio.Stdin = strings.NewReader("host=example.com\n")
	require.NoError(t, act.Get(c))
	read, err = parseGitCredentials(stdout)
	require.NoError(t, err)
	assert.Equal(t, "secr3=t", read.Password)
	assert.Equal(t, "bob", read.Username)
	stdout.Reset()

	termio.Stdin = strings.NewReader(s)
	require.NoError(t, act.Erase(c))
	assert.Empty(t, stdout.String())

	termio.Stdin = strings.NewReader(s)
	require.NoError(t, act.Get(c))
	assert.Empty(t, stdout.String())

	termio.Stdin = strings.NewReader("a")
	require.Error(t, act.Get(c))
	termio.Stdin = strings.NewReader("a")
	require.Error(t, act.Store(c))
	termio.Stdin = strings.NewReader("a")
	require.Error(t, act.Erase(c))
}

func TestGitCredentialHelperWithStoreFlag(t *testing.T) { //nolint:paralleltest
	ctx := t.Context()
	act := &gc{
		gp: apimock.New(),
	}

	stdout := &bytes.Buffer{}
	Stdout = stdout
	color.NoColor = true
	defer func() {
		Stdout = os.Stdout
		termio.Stdin = os.Stdin
	}()

	c := gptest.CliCtxWithFlags(ctx, t, map[string]string{
		"store": "teststore",
	})

	ctx = ctxutil.WithStdin(ctx, true)
	c.Context = ctx

	s := "protocol=https\n" +
		"host=example.com\n" +
		"username=bob\n"

	termio.Stdin = strings.NewReader(s)
	require.NoError(t, act.Get(c))
	assert.Empty(t, stdout.String())

	termio.Stdin = strings.NewReader(s + "password=secr3=t\n")
	require.NoError(t, act.Store(c))
	stdout.Reset()

	termio.Stdin = strings.NewReader(s)
	require.NoError(t, act.Get(c))
	read, err := parseGitCredentials(stdout)
	require.NoError(t, err)
	assert.Equal(t, "secr3=t", read.Password)
	stdout.Reset()

	c = gptest.CliCtxWithFlags(ctx, t, map[string]string{
		"store": "otherstore",
	})

	termio.Stdin = strings.NewReader(s)
	require.NoError(t, act.Get(c))
	assert.Empty(t, stdout.String())
}

func Test_getOptions(t *testing.T) {
	t.Parallel()

	type args struct {
		c *cli.Context
	}

	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name:    "without any flag",
			args:    args{c: gptest.CliCtxWithFlags(t.Context(), t, map[string]string{})},
			want:    []string{"config", "--global", "credential.helper", "gopass"},
			wantErr: false,
		},
		{
			name:    "with local scope flag",
			args:    args{c: gptest.CliCtxWithFlags(t.Context(), t, map[string]string{"local": "true"})},
			want:    []string{"config", "--local", "credential.helper", "gopass"},
			wantErr: false,
		},
		{
			name:    "with system scope flag",
			args:    args{c: gptest.CliCtxWithFlags(t.Context(), t, map[string]string{"system": "true"})},
			want:    []string{"config", "--system", "credential.helper", "gopass"},
			wantErr: false,
		},
		{
			name:    "with local scope flag and store",
			args:    args{c: gptest.CliCtxWithFlags(t.Context(), t, map[string]string{"local": "true", "store": "teststore"})},
			want:    []string{"config", "--local", "credential.helper", "gopass --store=teststore"},
			wantErr: false,
		},
		{
			name:    "error case with too many scope flags",
			args:    args{c: gptest.CliCtxWithFlags(t.Context(), t, map[string]string{"local": "true", "system": "true"})},
			want:    []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := getOptions(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOptions() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIntegration is a test for the integration of git-credential-gopass with a Git repository.
// It creates a temporary Git repository, sets up a remote, and tests the credential helper.
// First it tries to fetch from the remote without credentials, which should fail.
// Then it sets the credentials in the password store and tries to fetch again, which should succeed.
func TestIntegration(t *testing.T) {
	if !fsutil.IsFile("git-credential-gopass") || runtime.GOOS == "windows" {
		t.Skip("Skipping integration test, git-credential-gopass binary not found. Use make test to run unit tests.")
	}

	ctx := t.Context()

	// Create a temporary directory for the test
	td := t.TempDir()

	// Create a bin directory for the test
	binDir := filepath.Join(td, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))

	// Copy the credential helper binary to the bin directory.
	require.NoError(t, fsutil.CopyFile("git-credential-gopass", filepath.Join(binDir, "git-credential-gopass")))

	// Create a new Git repository in the temporary directory
	gitDir := filepath.Join(td, "test-repo")
	gitutils.InitGitDir(t, gitDir)

	// Create a new Git remote
	gitRemoteDir := filepath.Join(td, "test-remote.git")
	gitutils.InitGitBare(t, gitRemoteDir)

	// Create a password store
	gp := gptest.NewUnitTester(t)
	require.NoError(t, gp.InitStore(""))

	// Start the HTTP server
	srv := httptest.NewServer(githttp.BasicAuthMiddleware(githttp.GitHandler(td), "bob", "hunter2"))
	defer srv.Close()

	remoteURL := srv.URL + "/test-remote.git"
	// Add the remote to the Git repository
	cmd := exec.CommandContext(ctx, "git", "-C", gitDir, "remote", "add", "origin", remoteURL)
	require.NoError(t, cmd.Run())

	// Avoid asking for credentials
	cmd = exec.CommandContext(ctx, "git", "-C", gitDir, "config", "--local", "credential.interactive", "false")
	require.NoError(t, cmd.Run())

	// Set the credential helper to use gopass
	cmd = exec.CommandContext(ctx, "git", "-C", gitDir, "config", "--local", "credential.helper", "gopass")
	require.NoError(t, cmd.Run())

	// Do an initial fetch, it should fail because we don't have credentials yet.
	cmd = exec.CommandContext(ctx, "git", "-C", gitDir, "fetch", "origin")
	// Add the location of the helper binary to the PATH.
	cmd.Env = prependPath(t, os.Environ(), binDir)
	err := cmd.Run()
	require.Error(t, err, "fetch should fail without credentials")

	// Set credentials in the password store
	// URL is something like http://127.0.0.1:12345 and we need to store the secret as
	// `git/127.0.0.1_12345.txt`, i.e. first the `git/` prefix, then the URL with the port separated by `_`
	// and finally `.txt` suffix for the plaintext "encryption" the test helper uses.
	fn := filepath.Join(gp.StoreDir(""), "git", strings.ReplaceAll(strings.TrimPrefix(srv.URL, "http://"), ":", "_")+".txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(fn), 0o700))
	require.NoError(t, os.WriteFile(fn, []byte("hunter2\nlogin: bob\n"), 0o600))

	// Now fetch again, it should succeed
	cmd = exec.CommandContext(ctx, "git", "-C", gitDir, "fetch", "origin")
	// Add the location of the helper binary to the PATH.
	cmd.Env = prependPath(t, os.Environ(), binDir)
	require.NoError(t, cmd.Run(), "fetch should succeed with credentials")
}

func prependPath(t *testing.T, env []string, path string) []string {
	t.Helper()

	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + path + string(os.PathListSeparator) + e[5:]

			return env
		}
	}
	env = append(env, "PATH="+path)

	return env
}

func Test_composePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		credentials *gitCredentials
		store       string
		expected    string
	}{
		{
			name: "without repository path",
			credentials: &gitCredentials{
				Host:     "github.com",
				Username: "alice",
				Path:     "",
			},
			store:    "",
			expected: "git/github.com/alice",
		},
		{
			name: "with repository path",
			credentials: &gitCredentials{
				Host:     "github.com",
				Username: "alice",
				Path:     "repo1",
			},
			store:    "",
			expected: "git/github.com/repo1/alice",
		},
		{
			name: "with complex repository path",
			credentials: &gitCredentials{
				Host:     "github.com",
				Username: "alice",
				Path:     "user/myrepo.git",
			},
			store:    "",
			expected: "git/github.com/user_myrepo.git/alice",
		},
		{
			name: "with store prefix",
			credentials: &gitCredentials{
				Host:     "github.com",
				Username: "bob",
				Path:     "",
			},
			store:    "mystore",
			expected: "mystore/git/github.com/bob",
		},
		{
			name: "with store prefix and repository path",
			credentials: &gitCredentials{
				Host:     "github.com",
				Username: "bob",
				Path:     "repo2",
			},
			store:    "mystore",
			expected: "mystore/git/github.com/repo2/bob",
		},
		{
			name: "with special characters in host",
			credentials: &gitCredentials{
				Host:     "git.example.com",
				Username: "charlie",
				Path:     "",
			},
			store:    "",
			expected: "git/git.example.com/charlie",
		},
		{
			name: "multiple repos same host and user",
			credentials: &gitCredentials{
				Host:     "github.com",
				Username: "alice",
				Path:     "repo1",
			},
			store:    "",
			expected: "git/github.com/repo1/alice",
		},
		{
			name: "multiple repos same host and user alternate",
			credentials: &gitCredentials{
				Host:     "github.com",
				Username: "alice",
				Path:     "repo2",
			},
			store:    "",
			expected: "git/github.com/repo2/alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := gptest.CliCtxWithFlags(t.Context(), t, map[string]string{
				"store": tt.store,
			})

			got := composePath(c, tt.credentials)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestGitCredentialHelperMultipleCredentialsPerUser(t *testing.T) { //nolint:paralleltest
	ctx := t.Context()
	act := &gc{
		gp: apimock.New(),
	}

	stdout := &bytes.Buffer{}
	Stdout = stdout
	color.NoColor = true
	defer func() {
		Stdout = os.Stdout
		termio.Stdin = os.Stdin
	}()

	c := gptest.CliCtx(ctx, t)
	ctx = ctxutil.WithStdin(ctx, true)
	c.Context = ctx

	// Store first credential for myrepo
	s1 := "protocol=https\n" +
		"host=github.com\n" +
		"username=alice\n" +
		"path=repo1\n"

	termio.Stdin = strings.NewReader(s1 + "password=token1\n")
	require.NoError(t, act.Store(c))
	stdout.Reset()

	// Store second credential for myrepo-other (same user, same host, different path)
	s2 := "protocol=https\n" +
		"host=github.com\n" +
		"username=alice\n" +
		"path=repo2\n"

	termio.Stdin = strings.NewReader(s2 + "password=token2\n")
	require.NoError(t, act.Store(c))
	stdout.Reset()

	// Retrieve first credential
	termio.Stdin = strings.NewReader(s1)
	require.NoError(t, act.Get(c))
	read, err := parseGitCredentials(stdout)
	require.NoError(t, err)
	assert.Equal(t, "token1", read.Password)
	stdout.Reset()

	// Retrieve second credential - should get different token
	termio.Stdin = strings.NewReader(s2)
	require.NoError(t, act.Get(c))
	read, err = parseGitCredentials(stdout)
	require.NoError(t, err)
	assert.Equal(t, "token2", read.Password)
	stdout.Reset()

	// Erase first credential
	termio.Stdin = strings.NewReader(s1)
	require.NoError(t, act.Erase(c))
	stdout.Reset()

	// Try to retrieve first credential - should fail
	termio.Stdin = strings.NewReader(s1)
	require.NoError(t, act.Get(c))
	assert.Empty(t, stdout.String())

	// But second credential should still be available
	termio.Stdin = strings.NewReader(s2)
	require.NoError(t, act.Get(c))
	read, err = parseGitCredentials(stdout)
	require.NoError(t, err)
	assert.Equal(t, "token2", read.Password)
}

// TestIssue19_CredentialsInClonedRepo tests that credentials do not leak into the cloned repository.
// This is a regression test for https://github.com/gopasspw/git-credential-gopass/issues/19
// where credentials stored by git-credential-gopass would appear inside the cloned repository's
// working directory, which is a security issue.
func TestIssue19_CredentialsInClonedRepo(t *testing.T) {
	if !fsutil.IsFile("git-credential-gopass") || runtime.GOOS == "windows" {
		t.Skip("Skipping integration test, git-credential-gopass binary not found. Use make test to run unit tests.")
	}

	ctx := t.Context()

	// Create a temporary directory for the test
	td := t.TempDir()

	// Create a bin directory for the test
	binDir := filepath.Join(td, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))

	// Copy the credential helper binary to the bin directory
	require.NoError(t, fsutil.CopyFile("git-credential-gopass", filepath.Join(binDir, "git-credential-gopass")))

	// Create a password store in an isolated location
	gp := gptest.NewUnitTester(t)
	require.NoError(t, gp.InitStore(""))

	// Create a bare Git repository (remote)
	gitRemoteDir := filepath.Join(td, "remote.git")
	gitutils.InitGitBare(t, gitRemoteDir)

	// Start the HTTP server with Basic Auth
	srv := httptest.NewServer(githttp.BasicAuthMiddleware(githttp.GitHandler(td), "testuser", "testpass"))
	defer srv.Close()

	remoteURL := srv.URL + "/remote.git"

	// Create a directory for cloning
	cloneDir := filepath.Join(td, "cloned-repo")

	// Set up environment with the helper binary in PATH and gopass home directory
	env := prependPath(t, os.Environ(), binDir)
	// The gptest.NewUnitTester should already set GOPASS_HOMEDIR, but let's be explicit
	env = append(env, "GOPASS_HOMEDIR="+gp.Dir)

	// Configure git to use our credential helper and disable interactive mode
	// We'll do this in the clone command itself to avoid needing a pre-existing repo
	cloneCmd := exec.CommandContext(ctx, "git", "clone", remoteURL, cloneDir)
	cloneCmd.Env = env
	// Set global config for this test (will be isolated by HOME/XDG_CONFIG_HOME in env)
	configCmd := exec.CommandContext(ctx, "git", "config", "--global", "credential.helper", "gopass")
	configCmd.Env = env
	require.NoError(t, configCmd.Run(), "failed to configure credential helper")

	configCmd = exec.CommandContext(ctx, "git", "config", "--global", "credential.interactive", "false")
	configCmd.Env = env
	require.NoError(t, configCmd.Run(), "failed to disable interactive credentials")

	// Attempt to clone - this will fail because we don't have credentials yet,
	// but git-credential-gopass will be invoked and will fail to find credentials
	err := cloneCmd.Run()
	require.Error(t, err, "clone should fail without credentials")

	// Now store credentials in the gopass store
	// The URL is like http://127.0.0.1:12345 and we need to extract the host
	urlParts := strings.TrimPrefix(srv.URL, "http://")
	hostPort := strings.ReplaceAll(urlParts, ":", "_")

	// Store credential in gopass format: git/<host>/username
	credPath := filepath.Join(gp.StoreDir(""), "git", hostPort, "testuser.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	require.NoError(t, os.WriteFile(credPath, []byte("testpass\nlogin: testuser\n"), 0o600))

	// Now clone again - should succeed with credentials
	cloneCmd = exec.CommandContext(ctx, "git", "clone", remoteURL, cloneDir)
	cloneCmd.Env = env
	require.NoError(t, cloneCmd.Run(), "clone should succeed with credentials")

	// This is the key check for issue #19:
	// Verify that no credential files (.gpg or .txt) appear in the cloned repository
	var credentialFilesInRepo []string
	err = filepath.Walk(cloneDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir // Skip .git directory
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".gpg") ||
			(strings.HasSuffix(info.Name(), ".txt") && strings.Contains(path, "/git/"))) {
			credentialFilesInRepo = append(credentialFilesInRepo, path)
		}

		return nil
	})
	require.NoError(t, err, "failed to walk cloned repository")

	// Assert that no credential files leaked into the repository
	assert.Empty(t, credentialFilesInRepo,
		"Credential files should not appear in cloned repository (issue #19). Found: %v",
		credentialFilesInRepo)

	// Verify git status shows a clean working directory (no untracked files)
	statusCmd := exec.CommandContext(ctx, "git", "-C", cloneDir, "status", "--porcelain")
	statusCmd.Env = env
	output, err := statusCmd.Output()
	require.NoError(t, err, "git status should succeed")
	assert.Empty(t, strings.TrimSpace(string(output)),
		"Working directory should be clean with no untracked files. Got: %s", string(output))

	// Verify credentials are actually stored in gopass (not in the repo)
	require.FileExists(t, credPath, "Credentials should be stored in gopass store")
	assert.NotContains(t, credPath, cloneDir,
		"Credential path should not be inside the cloned repository")
}
