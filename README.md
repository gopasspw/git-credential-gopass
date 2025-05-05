# git-credential-gopass

This command allows you to cache your git-credentials with gopass.

## Pre-Installation Steps

If you want to use this helper, you should know, have installed and configured the password manager [Gopass].

You may also check if the helper is already installed.

```bash
$ git help -a | grep credential-
   credential-cache     Helper to temporarily store passwords in memory
   credential-store     Helper to store credentials on disk
   credential-gopass
```

After a successful installation and setting up in git you might find a line with `credential-gopass`.

## Installation Steps

Depending on your operating system, you can either download a pre-built binary, or install from source. If you have a working Go development environment, we recommend building from source.

### Alternative: Download

Find the appropriate package from the [releases] for your system and download it. Unpack the binary from the archive file. Move it to one of the locations listed in your $PATH.

Alternatively, use the installable version for your packet manager.

### Alternative: Building From Source

If you have [Go](https://golang.org/) already installed, you can use `go install` to automatically download the latest version:

```bash
go install  github.com/gopasspw/git-credential-gopass@latest
```

### Set Git Credential Helper

If `git-credential-gopass` is in your `$PATH`, you can now configure git.

```bash
git config --global credential.helper gopass
```

or

```bash
git-credential-gopass configure --<global|local|system>
```

For further git scoping details show up the documentation of [git credentials].

#### Option --store

You can save the credentials in a team store to share or manage a functional user for CI. Or just because you want it to.

```bash
git config credential.helper "gopass --store=ci-team"
```

```bash
git-credential-gopass configure --local --store=ci-team
```

This puts the value in front of the Gopass search path.

## Usage

After `git-credential-gopass` is set up it will be used by `git` to query and store credentials when they are needed.
Once you clone a Git repo that requires HTTP Authentication it will automatically create a new entry under the pattern
`git/HOST_PORT/REPO`. The secret must at least contain the password and the user like this:

```
Secret: git/localhost_8080

password
login: username
```

## Testing

If you don't have a password protected git repository available and don't want to use an SaaS provider like GitHub,
you can use the small helper tool that is included in this repository to set up a test environment.

Example usage:

```bash
$ mkdir -p /tmp/gitrepos
$ cd /tmp/gitrepos
$ git init --bare repo.git
$ go run helpers/githost/main.go -repo-root /tmp/gitrepos
$ git clone http://localhost:8080/repo.git
# Git will ask for username and password the first time you access this repo.
# Afterwards it will be cached and read from gopass.
```

## Links

[Gopass]: https://github.com/gopasspw/gopass
[releases]: https://github.com/gopasspw/git-credential-gopass/releases
[git credentials]: https://git-scm.com/docs/gitcredentials
