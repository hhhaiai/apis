#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BUILD_ADMIN_UI=1
INSTALL_ADMIN_DEPS=1
RUN_AFTER_BUILD=1
RUN_TESTS=0
PORT_VALUE="${PORT:-8080}"
GO_BINARY="bin/cc-gateway"

print_help() {
  cat <<'EOF'
Usage:
  scripts/build_run_gateway.sh [options]

Options:
  --no-ui               Skip building Vue admin UI
  --skip-npm-install    Skip npm install (assume deps already installed)
  --test                Run go test ./... before build
  --build-only          Build only, do not start gateway
  --port <PORT>         Set PORT when starting gateway (default: 8080)
  --binary <PATH>       Output binary path (default: bin/cc-gateway)
  -h, --help            Show this help

Environment:
  ADMIN_UI_DIST_DIR     Optional runtime dist dir override (served by /admin/)
  PORT                  Default port if --port is not provided
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-ui)
      BUILD_ADMIN_UI=0
      shift
      ;;
    --skip-npm-install)
      INSTALL_ADMIN_DEPS=0
      shift
      ;;
    --test)
      RUN_TESTS=1
      shift
      ;;
    --build-only)
      RUN_AFTER_BUILD=0
      shift
      ;;
    --port)
      PORT_VALUE="${2:-}"
      if [[ -z "$PORT_VALUE" ]]; then
        echo "error: --port requires a value" >&2
        exit 1
      fi
      shift 2
      ;;
    --binary)
      GO_BINARY="${2:-}"
      if [[ -z "$GO_BINARY" ]]; then
        echo "error: --binary requires a value" >&2
        exit 1
      fi
      shift 2
      ;;
    -h|--help)
      print_help
      exit 0
      ;;
    *)
      echo "error: unknown option: $1" >&2
      print_help >&2
      exit 1
      ;;
  esac
done

echo "== Build Config =="
echo "root:               $ROOT_DIR"
echo "build_admin_ui:     $BUILD_ADMIN_UI"
echo "install_admin_deps: $INSTALL_ADMIN_DEPS"
echo "run_tests:          $RUN_TESTS"
echo "run_after_build:    $RUN_AFTER_BUILD"
echo "go_binary:          $GO_BINARY"
echo "port:               $PORT_VALUE"
echo

if [[ "$BUILD_ADMIN_UI" -eq 1 ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    echo "error: npm not found; use --no-ui to skip UI build" >&2
    exit 1
  fi
  if [[ ! -f "web/admin/package.json" ]]; then
    echo "error: web/admin/package.json not found" >&2
    exit 1
  fi

  echo "== Build Admin UI =="
  if [[ "$INSTALL_ADMIN_DEPS" -eq 1 ]]; then
    npm --prefix web/admin install
  fi
  npm --prefix web/admin run build
  echo
fi

if [[ "$RUN_TESTS" -eq 1 ]]; then
  echo "== Go Tests =="
  go test ./...
  echo
fi

echo "== Build Gateway =="
mkdir -p "$(dirname "$GO_BINARY")"
go build -o "$GO_BINARY" ./cmd/cc-gateway
echo "built: $GO_BINARY"
echo

if [[ "$RUN_AFTER_BUILD" -eq 0 ]]; then
  echo "build-only mode: done"
  exit 0
fi

echo "== Start Gateway =="
export PORT="$PORT_VALUE"
exec "$GO_BINARY"
