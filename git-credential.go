package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/gopasspw/gopass/pkg/ctxutil"
	"github.com/gopasspw/gopass/pkg/debug"
	"github.com/gopasspw/gopass/pkg/fsutil"
	"github.com/gopasspw/gopass/pkg/gopass"
	"github.com/gopasspw/gopass/pkg/gopass/secrets"
	"github.com/gopasspw/gopass/pkg/termio"
	"github.com/urfave/cli/v2"
)

// Stdout is exported for tests.
var Stdout io.Writer = os.Stdout

type gitCredentials struct {
	Protocol          string
	Host              string
	Path              string
	Username          string
	Password          string
	PasswordExpiryUTC string
	OAuthRefreshToken string
}

// WriteTo writes the given credentials to the given io.Writer in the git-credential format.
func (c *gitCredentials) WriteTo(w io.Writer) (int64, error) {
	var n int64

	if c.Protocol != "" {
		i, err := io.WriteString(w, "protocol="+c.Protocol+"\n")
		n += int64(i)
		if err != nil {
			return n, err
		}
	}

	if c.Host != "" {
		i, err := io.WriteString(w, "host="+c.Host+"\n")
		n += int64(i)
		if err != nil {
			return n, err
		}
	}

	if c.Path != "" {
		i, err := io.WriteString(w, "path="+c.Path+"\n")
		n += int64(i)
		if err != nil {
			return n, err
		}
	}

	if c.Username != "" {
		i, err := io.WriteString(w, "username="+c.Username+"\n")
		n += int64(i)
		if err != nil {
			return n, err
		}
	}

	if c.Password != "" {
		i, err := io.WriteString(w, "password="+c.Password+"\n")
		n += int64(i)
		if err != nil {
			return n, err
		}
	}

	if c.PasswordExpiryUTC != "" {
		i, err := io.WriteString(w, "password_expiry_utc="+c.PasswordExpiryUTC+"\n")
		n += int64(i)
		if err != nil {
			return n, err
		}
	}

	if c.OAuthRefreshToken != "" {
		i, err := io.WriteString(w, "oauth_refresh_token="+c.OAuthRefreshToken+"\n")
		n += int64(i)
		if err != nil {
			return n, err
		}
	}

	return n, nil
}

func parseGitCredentials(r io.Reader) (*gitCredentials, error) {
	rd := bufio.NewReader(r)
	c := &gitCredentials{}
	for {
		key, err := rd.ReadString('=')
		if err != nil {
			if err == io.EOF {
				if key == "" {
					return c, nil
				}

				return nil, io.ErrUnexpectedEOF
			}

			return nil, err
		}

		key = strings.TrimSuffix(key, "=")
		val, err := rd.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}

			return nil, err
		}

		val = strings.TrimSuffix(val, "\n")
		switch key {
		case "protocol":
			c.Protocol = val
		case "host":
			c.Host = val
		case "path":
			c.Path = val
		case "username":
			c.Username = val
		case "password":
			c.Password = val
		case "password_expiry_utc":
			c.PasswordExpiryUTC = val
		case "oauth_refresh_token":
			c.OAuthRefreshToken = val
		}
	}
}

type gc struct {
	gp gopass.Store
}

// Before is executed before another git-credential command.
func (s *gc) Before(c *cli.Context) error {
	ctx := ctxutil.WithGlobalFlags(c)
	ctx = ctxutil.WithInteractive(ctx, false)
	if !ctxutil.IsStdin(ctx) {
		return fmt.Errorf("missing stdin from git")
	}

	return nil
}

func filter(ls []string, prefix string) []string {
	out := make([]string, 0, len(ls))
	for _, e := range ls {
		if !strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}

	return out
}

func composePath(c *cli.Context, cred *gitCredentials) string {
	var gopassPath string

	if c.String("store") != "" {
		gopassPath = path.Join(c.String("store"))
	}

	gopassPath = path.Join(gopassPath, "git", fsutil.CleanFilename(cred.Host))

	// If path is supplied due to useHttpPath being set,
	// assume the first part of the path is a user or organization.
	if cred.Path != "" {
		parts := strings.Split(cred.Path, string(os.PathSeparator))
		if len(parts) > 0 {
			gopassPath = path.Join(gopassPath, parts[0])
		}
	}

	gopassPath = path.Join(gopassPath, fsutil.CleanFilename(cred.Username))

	return gopassPath
}

// Get returns a credential to git.
func (s *gc) Get(c *cli.Context) error {
	ctx := ctxutil.WithGlobalFlags(c)
	ctx = ctxutil.WithNoNetwork(ctx, true)
	cred, err := parseGitCredentials(termio.Stdin)
	if err != nil {
		return fmt.Errorf("error: %w while parsing git-credential", err)
	}
	// try git/host/username... If username is empty, simply try git/host

	path := composePath(c, cred)
	if _, err := s.gp.Get(ctx, path, "latest"); err != nil {
		// if the looked up path is a directory with only one entry (e.g. one user per host), take the subentry instead
		ls, err := s.gp.List(ctx)
		if err != nil {
			return fmt.Errorf("error: %w while listing the storage", err)
		}
		entries := filter(ls, path)
		if len(entries) < 1 {
			// no entry found, this is not an error
			return nil
		}
		if len(entries) > 1 {
			fmt.Fprintln(os.Stderr, "gopass error: too many entries")

			return nil
		}
		path = entries[0]
	}
	secret, err := s.gp.Get(ctx, path, "latest")
	if err != nil {
		return err
	}

	cred.Password = secret.Password()
	if username, _ := secret.Get("login"); username != "" {
		// leave the username as is otherwise
		cred.Username = username
	}
	if expiry, _ := secret.Get("password_expiry_utc"); expiry != "" {
		cred.PasswordExpiryUTC = expiry
	}
	if rt, _ := secret.Get("oauth_refresh_token"); rt != "" {
		cred.OAuthRefreshToken = rt
	}

	_, err = cred.WriteTo(Stdout)
	if err != nil {
		return fmt.Errorf("could not write to stdout: %w", err)
	}

	return nil
}

// Store stores a credential got from git.
func (s *gc) Store(c *cli.Context) error {
	ctx := ctxutil.WithGlobalFlags(c)
	cred, err := parseGitCredentials(termio.Stdin)
	if err != nil {
		return fmt.Errorf("error: %w while parsing git-credential", err)
	}

	path := composePath(c, cred)
	// This should never really be an issue because git automatically removes invalid credentials first
	if _, err := s.gp.Get(ctx, path, "latest"); err == nil {
		debug.Log(""+
			"gopass: did not store \"%s\" because it already exists. "+
			"If you want to overwrite it, delete it first by doing: "+
			"\"gopass rm %s\"\n",
			path, path,
		)

		return nil
	}
	secret := secrets.New()
	secret.SetPassword(cred.Password)
	if cred.Username != "" {
		_ = secret.Set("login", cred.Username)
	}
	if cred.PasswordExpiryUTC != "" {
		_ = secret.Set("password_expiry_utc", cred.PasswordExpiryUTC)
	}
	if cred.OAuthRefreshToken != "" {
		_ = secret.Set("oauth_refresh_token", cred.OAuthRefreshToken)
	}

	if err := s.gp.Set(ctx, path, secret); err != nil {
		fmt.Fprintf(os.Stderr, "gopass error: error while writing to store: %s\n", err)
	}

	return nil
}

// Erase removes a credential got from git.
func (s *gc) Erase(c *cli.Context) error {
	ctx := ctxutil.WithGlobalFlags(c)
	cred, err := parseGitCredentials(termio.Stdin)
	if err != nil {
		return fmt.Errorf("error: %w while parsing git-credential", err)
	}

	path := composePath(c, cred)
	if err := s.gp.Remove(ctx, path); err != nil {
		fmt.Fprintln(os.Stderr, "gopass error: error while writing to store")
	}

	return nil
}

// Configure configures gopass as git's credential.helper.
func (s *gc) Configure(c *cli.Context) error {
	ctx := ctxutil.WithGlobalFlags(c)
	options, err := getOptions(c)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", options...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func getOptions(c *cli.Context) ([]string, error) {
	options := []string{}
	flags := 0
	flag := "--global"
	if c.Bool("local") {
		flag = "--local"
		flags++
	}

	if c.Bool("global") {
		flag = "--global"
		flags++
	}

	if c.Bool("system") {
		flag = "--system"
		flags++
	}

	if flags >= 2 {
		return options, fmt.Errorf("only specify one target of installation")
	}

	if flags == 0 {
		log.Println("No target given, assuming --global.")
	}

	options = append(options, "config", flag, "credential.helper")
	store := "gopass"
	if s := c.String("store"); s != "" {
		store = fmt.Sprintf("gopass --store=%s", s)
	}

	options = append(options, store)

	return options, nil
}
