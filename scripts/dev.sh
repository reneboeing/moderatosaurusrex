#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root_dir"

state_dir=.dev
binary="$state_dir/moderatosaurusrex"
pid_file="$state_dir/bot.pid"
log_file="$state_dir/bot.log"

running_pid() {
  if [[ ! -s "$pid_file" ]]; then
    return 1
  fi

  local pid
  pid=$(<"$pid_file")
  if kill -0 "$pid" 2>/dev/null; then
    printf '%s\n' "$pid"
    return 0
  fi

  rm -f "$pid_file"
  return 1
}

stop() {
  local pid
  if ! pid=$(running_pid); then
    echo "Bot is not running."
    return
  fi

  kill -TERM "$pid"
  for _ in {1..20}; do
    if ! kill -0 "$pid" 2>/dev/null; then
      rm -f "$pid_file"
      echo "Bot stopped."
      return
    fi
    sleep 0.1
  done

  echo "Bot did not stop after two seconds; send make dev-down again."
  return 1
}

start() {
  local pid
  if pid=$(running_pid); then
    echo "Bot is already running (PID $pid)."
    return
  fi

  if [[ ! -f .env ]]; then
    echo "Missing .env. Copy .env.example and fill in the Discord and database values." >&2
    return 1
  fi

  mkdir -p "$state_dir"
  go build -o "$binary" ./cmd/moderatosaurusrex
  : >"$log_file"
  set -a
  # shellcheck source=/dev/null
  . ./.env
  set +a
  nohup "$binary" >>"$log_file" 2>&1 < /dev/null &
  pid=$!
  printf '%s\n' "$pid" >"$pid_file"
  sleep 0.2

  if ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$pid_file"
    cat "$log_file" >&2
    return 1
  fi

  echo "Bot started (PID $pid). Logs: $log_file"
}

case "${1:-}" in
  up) start ;;
  down) stop ;;
  restart) stop; start ;;
  status)
    if pid=$(running_pid); then
      echo "Bot is running (PID $pid). Logs: $log_file"
    else
      echo "Bot is not running."
    fi
    ;;
  logs)
    mkdir -p "$state_dir"
    touch "$log_file"
    tail -n 100 -f "$log_file"
    ;;
  *)
    echo "Usage: $0 {up|down|restart|status|logs}" >&2
    exit 2
    ;;
esac
