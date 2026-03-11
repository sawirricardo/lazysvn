#!/usr/bin/env sh
set -eu

BINARY_NAME="lazysvn"
REPO="${LAZYSVN_REPO:-sawirricardo/lazysvn}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"

usage() {
  cat <<EOF
LazySVN installer

Usage:
  curl -fsSL https://lazysvn.sawirstudio.com/install.sh | sh

Options via environment variables:
  VERSION=<tag>            Install a specific release tag (default: latest)
  INSTALL_DIR=<path>       Install directory (default: \$HOME/.local/bin)
  LAZYSVN_REPO=<owner/repo>  Release source repo (default: sawirricardo/lazysvn)
  GITHUB_TOKEN=<token>     Optional token to avoid GitHub API rate limits
EOF
}

log() {
  printf '==> %s\n' "$1"
}

fail() {
  printf 'error: %s\n' "$1" >&2
  exit 1
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

detect_os() {
  case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
    linux*) printf 'linux' ;;
    darwin*) printf 'darwin' ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

find_asset_url() {
  urls="$1"
  os="$2"
  arch="$3"

  # Preferred pattern: lazysvn_<version>_<os>_<arch>.tar.gz
  exact="$(printf '%s\n' "$urls" \
    | grep -E "/${BINARY_NAME}_[vV]?[0-9][^/]*_${os}_${arch}\.(tar\.gz|tgz|zip)$|/${BINARY_NAME}_${os}_${arch}\.(tar\.gz|tgz|zip)$" \
    | head -n 1 || true)"
  if [ -n "$exact" ]; then
    printf '%s' "$exact"
    return 0
  fi

  # Fallback pattern if naming differs but still includes os+arch.
  loose="$(printf '%s\n' "$urls" \
    | grep -E "/${BINARY_NAME}.*${os}.*${arch}.*\.(tar\.gz|tgz|zip)$" \
    | head -n 1 || true)"
  if [ -n "$loose" ]; then
    printf '%s' "$loose"
    return 0
  fi

  return 1
}

extract_asset_urls_from_json() {
  json="$1"
  printf '%s' "$json" \
    | tr ',' '\n' \
    | sed -n 's/.*"browser_download_url":[[:space:]]*"\([^"]*\)".*/\1/p'
}

extract_asset_urls_from_html() {
  html="$1"
  printf '%s' "$html" \
    | tr '"' '\n' \
    | sed -n 's#^/\([^"]*/releases/download/[^"]*\)$#https://github.com/\1#p'
}

github_api_get() {
  url="$1"

  if [ -n "$GITHUB_TOKEN" ]; then
    curl -fsSL \
      -H "Accept: application/vnd.github+json" \
      -H "Authorization: Bearer ${GITHUB_TOKEN}" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      -H "User-Agent: lazysvn-installer" \
      "$url" 2>/dev/null || true
    return 0
  fi

  curl -fsSL \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    -H "User-Agent: lazysvn-installer" \
    "$url" 2>/dev/null || true
}

github_web_get() {
  url="$1"
  curl -fsSL \
    -H "User-Agent: lazysvn-installer" \
    "$url" 2>/dev/null || true
}

resolve_latest_release_tag() {
  final_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest" 2>/dev/null || true)"
  [ -n "$final_url" ] || return 1
  tag="$(printf '%s' "$final_url" | sed -n 's#.*/releases/tag/\([^/?#]*\).*#\1#p')"
  [ -n "$tag" ] || return 1
  printf '%s' "$tag"
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ] || [ "${1:-}" = "help" ]; then
  usage
  exit 0
fi

if [ -n "${1:-}" ]; then
  usage
  fail "unknown argument: $1"
fi

need_cmd curl
need_cmd uname
need_cmd tr
need_cmd sed
need_cmd grep
need_cmd head
need_cmd mktemp
need_cmd mkdir
need_cmd chmod
need_cmd tar
need_cmd find

os="$(detect_os)"
arch="$(detect_arch)"

api_base="https://api.github.com/repos/${REPO}/releases"
if [ "$VERSION" = "latest" ]; then
  release_api_url="${api_base}/latest"
else
  release_api_url="${api_base}/tags/${VERSION}"
fi

log "Resolving ${REPO} release (${VERSION}) for ${os}/${arch}"
release_urls=""
asset_url=""

release_json="$(github_api_get "$release_api_url")"
if [ -n "$release_json" ]; then
  release_urls="$(extract_asset_urls_from_json "$release_json")"
  asset_url="$(find_asset_url "$release_urls" "$os" "$arch" || true)"
fi

if [ -z "$asset_url" ]; then
  if [ "$VERSION" = "latest" ]; then
    resolved_version="$(resolve_latest_release_tag || true)"
  else
    resolved_version="$VERSION"
  fi

  if [ -n "${resolved_version:-}" ]; then
    release_html="$(github_web_get "https://github.com/${REPO}/releases/expanded_assets/${resolved_version}")"
    if [ -n "$release_html" ]; then
      release_urls="$(extract_asset_urls_from_html "$release_html")"
      asset_url="$(find_asset_url "$release_urls" "$os" "$arch" || true)"
    fi
  fi
fi

tmp_dir="$(mktemp -d 2>/dev/null || mktemp -d -t lazysvn)"
archive_file="${tmp_dir}/asset"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

if [ -n "$asset_url" ]; then
  log "Downloading ${asset_url}"
  curl -fL "$asset_url" -o "$archive_file"

  case "$asset_url" in
    *.tar.gz|*.tgz)
      tar -xzf "$archive_file" -C "$tmp_dir"
      ;;
    *.zip)
      need_cmd unzip
      unzip -q "$archive_file" -d "$tmp_dir"
      ;;
    *)
      fail "unsupported archive format in asset URL: ${asset_url}"
      ;;
  esac

  bin_path="$(find "$tmp_dir" -type f -name "$BINARY_NAME" | head -n 1 || true)"
  [ -n "$bin_path" ] || fail "binary '${BINARY_NAME}' not found in release archive"
else
  fail "no prebuilt asset found for ${REPO} (${VERSION}) on ${os}/${arch}"
fi

mkdir -p "$INSTALL_DIR"
target="${INSTALL_DIR}/${BINARY_NAME}"

if command -v install >/dev/null 2>&1; then
  install -m 0755 "$bin_path" "$target"
else
  cp "$bin_path" "$target"
  chmod 0755 "$target"
fi

log "Installed ${BINARY_NAME} to ${target}"
if "$target" --help >/dev/null 2>&1; then
  log "Run: ${target}"
fi

case ":$PATH:" in
  *":${INSTALL_DIR}:"*)
    ;;
  *)
    log "Add to PATH if needed: export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac
