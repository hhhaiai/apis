#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BUILD_ADMIN_UI=1
INSTALL_ADMIN_DEPS=1
RUN_AFTER_BUILD=1
RUN_TESTS=0
SKIP_BUILD=0
DETACHED=0
STOP_ONLY=0
PORT_VALUE="${PORT:-8080}"
GO_BINARY="bin/cc-gateway"
LOG_FILE="${LOG_FILE:-logs/cc-gateway.log}"
PID_FILE="${PID_FILE:-logs/cc-gateway.pid}"

print_help() {
  cat <<'EOF'
Usage:
  scripts/build_run_gateway.sh [options]

Options:
  --quick               Fast mode: skip npm install + tests
  --no-ui               Skip building Vue admin UI
  --skip-npm-install    Skip npm install (assume deps already installed)
  --test                Run go test ./... before build
  --build-only          Build only, do not start gateway
  --no-build            Skip build phase and start existing binary directly
  --detached            Start gateway in background and write PID/log
  --stop                Stop detached gateway process and exit
  --port <PORT>         Set PORT when starting gateway (default: 8080)
  --binary <PATH>       Output binary path (default: bin/cc-gateway)
  --log-file <PATH>     Detached mode log file (default: logs/cc-gateway.log)
  --pid-file <PATH>     Detached mode pid file (default: logs/cc-gateway.pid)
  -h, --help            Show this help

Environment:
  ADMIN_UI_DIST_DIR     Optional runtime dist dir override (served by /admin/)
  PORT                  Default port if --port is not provided
EOF
}

is_process_running() {
  local pid="$1"
  if [[ -z "$pid" ]]; then
    return 1
  fi
  kill -0 "$pid" >/dev/null 2>&1
}

stop_detached() {
  if [[ ! -f "$PID_FILE" ]]; then
    echo "no pid file found: $PID_FILE"
    return 0
  fi
  local pid
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  if ! is_process_running "$pid"; then
    echo "stale pid file removed: $PID_FILE"
    rm -f "$PID_FILE"
    return 0
  fi
  echo "stopping gateway pid=$pid ..."
  kill "$pid" >/dev/null 2>&1 || true
  for _ in {1..10}; do
    if ! is_process_running "$pid"; then
      break
    fi
    sleep 0.3
  done
  if is_process_running "$pid"; then
    echo "force killing gateway pid=$pid ..."
    kill -9 "$pid" >/dev/null 2>&1 || true
  fi
  rm -f "$PID_FILE"
  echo "gateway stopped"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quick)
      INSTALL_ADMIN_DEPS=0
      RUN_TESTS=0
      shift
      ;;
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
    --no-build)
      SKIP_BUILD=1
      shift
      ;;
    --detached)
      DETACHED=1
      shift
      ;;
    --stop)
      STOP_ONLY=1
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
    --log-file)
      LOG_FILE="${2:-}"
      if [[ -z "$LOG_FILE" ]]; then
        echo "error: --log-file requires a value" >&2
        exit 1
      fi
      shift 2
      ;;
    --pid-file)
      PID_FILE="${2:-}"
      if [[ -z "$PID_FILE" ]]; then
        echo "error: --pid-file requires a value" >&2
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

if [[ "$STOP_ONLY" -eq 1 ]]; then
  stop_detached
  exit 0
fi

echo "== Build Config =="
echo "root:               $ROOT_DIR"
echo "build_admin_ui:     $BUILD_ADMIN_UI"
echo "install_admin_deps: $INSTALL_ADMIN_DEPS"
echo "run_tests:          $RUN_TESTS"
echo "skip_build:         $SKIP_BUILD"
echo "run_after_build:    $RUN_AFTER_BUILD"
echo "detached:           $DETACHED"
echo "go_binary:          $GO_BINARY"
echo "port:               $PORT_VALUE"
echo "log_file:           $LOG_FILE"
echo "pid_file:           $PID_FILE"
echo

if [[ "$SKIP_BUILD" -eq 0 ]]; then
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
    elif [[ ! -d "web/admin/node_modules" ]]; then
      echo "warning: web/admin/node_modules missing; running npm install once"
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
fi

if [[ "$RUN_AFTER_BUILD" -eq 0 ]]; then
  echo "build-only mode: done"
  exit 0
fi

if [[ ! -x "$GO_BINARY" ]]; then
  echo "error: binary not found or not executable: $GO_BINARY" >&2
  exit 1
fi

export PORT="$PORT_VALUE"

if [[ "$DETACHED" -eq 1 ]]; then
  echo "== Start Gateway (Detached) =="
  mkdir -p "$(dirname "$LOG_FILE")"
  mkdir -p "$(dirname "$PID_FILE")"
  stop_detached
  nohup "$GO_BINARY" >"$LOG_FILE" 2>&1 &
  pid="$!"
  echo "$pid" >"$PID_FILE"
  sleep 1
  if ! is_process_running "$pid"; then
    echo "gateway failed to start, check log: $LOG_FILE" >&2
    tail -n 80 "$LOG_FILE" || true
    exit 1
  fi
  echo "gateway started in background"
  echo "pid:  $pid"
  echo "log:  $LOG_FILE"
  echo "url:  http://127.0.0.1:${PORT_VALUE}/"
  echo "admin:http://127.0.0.1:${PORT_VALUE}/admin/"
  exit 0
fi

echo "== Start Gateway =="
exec "$GO_BINARY"
