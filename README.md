# LazySVN

A lightweight, LazyGit-style terminal UI for Subversion (SVN), written in Go.

## Quick install (curl | sh)

```bash
curl -fsSL https://lazysvn.sawirstudio.com/install.sh | sh
```

Optional:

```bash
# Install to custom dir
curl -fsSL https://lazysvn.sawirstudio.com/install.sh | INSTALL_DIR=/usr/local/bin sh

# Install specific tag
curl -fsSL https://lazysvn.sawirstudio.com/install.sh | VERSION=v0.1.0 sh
```

`install.sh` installs a matching prebuilt binary from GitHub Releases for your OS/architecture.
If no matching binary exists for your platform, install exits with an error.

After install, you can self-update from the CLI:

```bash
lazysvn update
# or pin to a specific tag
lazysvn update v0.1.0
```

## What it does

- Shows `svn status` in a navigable list
- Shows `svn diff` for the selected file
- Shows `svn log -l 20` for the selected file
- Marks multiple files for actions
- Runs `svn commit`, `svn add`, `svn revert`, and `svn update`

## Run

```bash
go mod tidy
go run .
```

Run it from inside an SVN working copy.

## Homebrew install

You can install via a Homebrew tap.

1. Create a tap repo (once), e.g. `sawirricardo/homebrew-tap`
2. Add the formula from [`packaging/homebrew/lazysvn.rb`](packaging/homebrew/lazysvn.rb) to that tap as `Formula/lazysvn.rb`
3. Install:

```bash
brew tap sawirricardo/tap
brew install --HEAD lazysvn
```

Then run from any SVN working copy:

```bash
lazysvn
```

## Uninstall

From the CLI:

```bash
lazysvn uninstall
```

If you installed with `curl | sh`, remove the binary from your install directory:

```bash
rm -f "$HOME/.local/bin/lazysvn"
```

If you installed to a custom directory, remove that binary path instead.

If you installed with Homebrew:

```bash
brew uninstall lazysvn
```

## Keybindings

- `j` / `k` or arrows: move
- `g` / `G`: top / bottom
- `space`: mark/unmark file
- `d` or `Enter`: diff selected file
- `l`: show log for selected file
- `c`: commit marked files (or current file)
- `a`: add marked/selected unversioned files (`?`)
- `v`: revert marked/current file (with confirmation)
- `u`: update working copy
- `r`: refresh status
- `h` or `?`: help
- `q`: quit
