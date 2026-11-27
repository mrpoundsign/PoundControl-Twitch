#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-all}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLIENT_ID="${TWITCH_CLIENT_ID:-}"
VERSION="${VERSION:-0.1.1}"

if command -v git &>/dev/null && [ -d "$ROOT_DIR/.git" ] && [ -d "$ROOT_DIR/.githooks" ]; then
    git -C "$ROOT_DIR" config core.hooksPath .githooks 2>/dev/null || true
fi

for dir in "$HOME/.local/bin" "$HOME/go/bin" "/usr/local/go/bin" "$HOME/.asdf/shims" "$HOME/.asdf/bin" "/home/linuxbrew/.linuxbrew/bin" "/home/linuxbrew/.linuxbrew/sbin"; do
    if [ -d "$dir" ] && [[ ":$PATH:" != *":$dir:"* ]]; then
        export PATH="$dir:$PATH"
    fi
done

run_lint() {
    echo "==> go fix -diff ./..."
    diff_out="$(go fix -diff ./...)"
    if [[ -n "$diff_out" ]]; then
        echo "$diff_out"
        exit 1
    fi
    echo "==> go vet ./..."
    go vet ./...
    if command -v golangci-lint-v2 >/dev/null 2>&1; then
        echo "==> golangci-lint-v2 run ./..."
        golangci-lint-v2 run ./...
    fi
}

run_test() {
    echo "==> go test -race -count=1 ./..."
    go test -race -count=1 ./...
}

run_wasm() {
    mkdir -p build/bin
    ldflags="-s -w -X github.com/mrpoundsign/PoundControl-Twitch/internal/twitch.PluginVersion=${VERSION}"
    if [[ -n "$CLIENT_ID" ]]; then
        ldflags="${ldflags} -X github.com/mrpoundsign/PoundControl-Twitch/internal/twitch.DefaultClientID=${CLIENT_ID}"
    fi
    echo "==> Building build/bin/poundcontrol-twitch.wasm (wasip1/wasm reactor)"
    GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -ldflags "$ldflags" -o build/bin/poundcontrol-twitch.wasm .
    ls -lh build/bin/poundcontrol-twitch.wasm
}

case "$TARGET" in
    lint) run_lint ;;
    test) run_test ;;
    wasm) run_wasm ;;
    all)
        run_lint
        run_test
        run_wasm
        ;;
    *)
        echo "Usage: $0 [all|lint|test|wasm]" >&2
        exit 1
        ;;
esac
