#!/usr/bin/env bash
# Idempotent Cloud Agent setup for the Sightmap monorepo.
# Installs the toolchains the repo needs beyond the base image (a modern Go and
# the Mintlify CLI) and bootstraps dependencies for every area: root release
# tooling, the web app, and the spec validators. Safe to run repeatedly.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# --- Go toolchain -----------------------------------------------------------
# go/go.mod requires go >= 1.23 (README asks for 1.25.2+); the base image ships
# 1.22 and GOTOOLCHAIN auto-download is unavailable, so install a pinned Go.
GO_VERSION="1.25.2"
NEED_GO=1
if command -v go >/dev/null 2>&1 && go version | grep -q "go${GO_VERSION}"; then
  NEED_GO=0
fi
if [ "$NEED_GO" -eq 1 ]; then
  echo "install: installing Go ${GO_VERSION}"
  curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf /tmp/go.tgz
  rm -f /tmp/go.tgz
  # Base image resolves `go` from /usr/bin (older apt Go). /usr/local/bin wins
  # in PATH, so point it at the freshly installed toolchain.
  sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
  sudo ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
fi
hash -r
go version

# --- Node global prefix + Mintlify CLI --------------------------------------
# The base image's npm prefix is `/`, which needs root for `-g` installs. Point
# global installs at a user-writable prefix and put it on PATH for later shells.
NPM_GLOBAL="$HOME/.npm-global"
mkdir -p "$NPM_GLOBAL"
npm config set prefix "$NPM_GLOBAL"
if ! grep -q '.npm-global/bin' "$HOME/.bashrc" 2>/dev/null; then
  echo 'export PATH="$HOME/.npm-global/bin:$PATH"' >> "$HOME/.bashrc"
fi
export PATH="$NPM_GLOBAL/bin:$PATH"
if ! command -v mint >/dev/null 2>&1; then
  echo "install: installing Mintlify CLI"
  npm install -g mint
fi
mint --version

# --- Dependencies -----------------------------------------------------------
echo "install: root npm ci (changesets/jest/prettier tooling)"
npm ci

echo "install: web deps (pnpm)"
corepack enable >/dev/null 2>&1 || true
cd web
pnpm install --frozen-lockfile
# pnpm does not run esbuild's install script by default; build its native
# binary so tsx/vite work.
pnpm rebuild esbuild
cd "$REPO_ROOT"

echo "install: spec validators (npm ci)"
cd spec
npm ci
cd "$REPO_ROOT"

echo "install: warming the Go module cache + building the CLI"
cd go
go build ./...
cd "$REPO_ROOT"

echo "install: done"
