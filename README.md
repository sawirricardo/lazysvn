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
If no matching binary exists yet, it falls back to building from source (requires `go`).

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

## Hosting install.sh

Deployable static assets live in [`static/`](static):

- [`static/index.html`](static/index.html)
- [`static/install.sh`](static/install.sh)

To serve from your domain, host that folder with:

- `https://lazysvn.sawirstudio.com/` -> `static/index.html`
- `https://lazysvn.sawirstudio.com/install.sh` -> `static/install.sh`

Cloudflare Pages settings:

- Framework preset: `None`
- Build command: *(empty)*
- Build output directory: `static`

## GitHub releases automation

GitHub Actions workflow: [`.github/workflows/release.yml`](.github/workflows/release.yml)

- Push a tag like `v0.1.0` to trigger release builds automatically
- Or run it manually from Actions using `workflow_dispatch` with a `tag`
- It publishes archives for:
  - `linux/amd64`
  - `linux/arm64`
  - `darwin/amd64`
  - `darwin/arm64`
- Asset names follow: `lazysvn_<tag>_<os>_<arch>.tar.gz` (compatible with `install.sh`)

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
