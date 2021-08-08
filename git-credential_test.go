package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/gopasspw/gopass/pkg/ctxutil"
	"github.com/gopasspw/gopass/pkg/gopass/apimock"
	"github.com/gopasspw/gopass/pkg/termio"
	"github.com/gopasspw/gopass/tests/gptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestGitCredentialFormat(t *testing.T) {
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
		{},
		{},
	}
	expectsErr := []bool{false, true, true}
	for i := range data {
		result, err := parseGitCredentials(data[i])
		if expectsErr[i] {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		if err != nil {
			continue
		}
		assert.Equal(t, results[i], *result)
		buf := &bytes.Buffer{}
		n, err := result.WriteTo(buf)
		assert.NoError(t, err, "could not serialize credentials")
		assert.Equal(t, buf.Len(), int(n))
		parseback, err := parseGitCredentials(buf)
		assert.NoError(t, err, "failed parsing my own output")
		assert.Equal(t, results[i], *parseback, "failed parsing my own output")
	}
}

func TestGitCredentialHelper(t *testing.T) {
	ctx := context.Background()
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
	assert.Error(t, act.Before(c))

	// before with stdin
	ctx = ctxutil.WithStdin(ctx, true)
	c.Context = ctx
	assert.NoError(t, act.Before(c))

	s := "protocol=https\n" +
		"host=example.com\n" +
		"username=bob\n"

	termio.Stdin = strings.NewReader(s)
	assert.NoError(t, act.Get(c))
	assert.Equal(t, "", stdout.String())

	termio.Stdin = strings.NewReader(s + "password=secr3=t\n")
	assert.NoError(t, act.Store(c))
	stdout.Reset()

	termio.Stdin = strings.NewReader(s)
	assert.NoError(t, act.Get(c))
	read, err := parseGitCredentials(stdout)
	assert.NoError(t, err)
	assert.Equal(t, "secr3=t", read.Password)
	stdout.Reset()

	termio.Stdin = strings.NewReader("host=example.com\n")
	assert.NoError(t, act.Get(c))
	read, err = parseGitCredentials(stdout)
	assert.NoError(t, err)
	assert.Equal(t, "secr3=t", read.Password)
	assert.Equal(t, "bob", read.Username)
	stdout.Reset()

	termio.Stdin = strings.NewReader(s)
	assert.NoError(t, act.Erase(c))
	assert.Equal(t, "", stdout.String())

	termio.Stdin = strings.NewReader(s)
	assert.NoError(t, act.Get(c))
	assert.Equal(t, "", stdout.String())

	termio.Stdin = strings.NewReader("a")
	assert.Error(t, act.Get(c))
	termio.Stdin = strings.NewReader("a")
	assert.Error(t, act.Store(c))
	termio.Stdin = strings.NewReader("a")
	assert.Error(t, act.Erase(c))
}

func TestGitCredentialHelperWithStoreFlag(t *testing.T) {
	ctx := context.Background()
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
	assert.NoError(t, act.Get(c))
	assert.Equal(t, "", stdout.String())

	termio.Stdin = strings.NewReader(s + "password=secr3=t\n")
	assert.NoError(t, act.Store(c))
	stdout.Reset()

	termio.Stdin = strings.NewReader(s)
	assert.NoError(t, act.Get(c))
	read, err := parseGitCredentials(stdout)
	assert.NoError(t, err)
	assert.Equal(t, "secr3=t", read.Password)
	stdout.Reset()

	c = gptest.CliCtxWithFlags(ctx, t, map[string]string{
		"store": "otherstore",
	})

	termio.Stdin = strings.NewReader(s)
	assert.NoError(t, act.Get(c))
	assert.Equal(t, "", stdout.String())

}

func Test_getOptions(t *testing.T) {
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
			args:    args{c: gptest.CliCtxWithFlags(context.Background(), t, map[string]string{})},
			want:    []string{"config", "--global", "credential.helper", "gopass"},
			wantErr: false,
		},
		{
			name:    "with local scope flag",
			args:    args{c: gptest.CliCtxWithFlags(context.Background(), t, map[string]string{"local": "true"})},
			want:    []string{"config", "--local", "credential.helper", "gopass"},
			wantErr: false,
		},
		{
			name:    "with system scope flag",
			args:    args{c: gptest.CliCtxWithFlags(context.Background(), t, map[string]string{"system": "true"})},
			want:    []string{"config", "--system", "credential.helper", "gopass"},
			wantErr: false,
		},
		{
			name:    "with local scope flag and store",
			args:    args{c: gptest.CliCtxWithFlags(context.Background(), t, map[string]string{"local": "true", "store": "teststore"})},
			want:    []string{"config", "--local", "credential.helper", "gopass --store=teststore"},
			wantErr: false,
		},
		{
			name:    "error case with too many scope flags",
			args:    args{c: gptest.CliCtxWithFlags(context.Background(), t, map[string]string{"local": "true", "system": "true"})},
			want:    []string{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOptions(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
