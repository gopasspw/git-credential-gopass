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

#### Using with SMTP

If you want to use this with [`git-send-email`](https://git-scm.com/docs/git-send-email) you'll need to:

1. [Set Git Credential Helper](#set-git-credential-helper)
2. Put your secret (encrypted) in `git/example.com_465/username.gpg`. It is important that the host, top level domain, port and username are there.
3. Include an [`[sendemail]`](https://git-scm.com/docs/git-send-email#_examples) in your `.gitconfig` or alternatively `.git/config`. Remember to prefer `smtpEncryption = ssl` and `smtpServerPort = 465` if your provider supports [implicit TLS](https://git-scm.com/docs/git-send-email#Documentation/git-send-email.txt---smtp-encryptionltencryptiongt).

[Gopass]: https://github.com/gopasspw/gopass
[releases]: https://github.com/gopasspw/git-credential-gopass/releases
[git credentials]: https://git-scm.com/docs/gitcredentials
