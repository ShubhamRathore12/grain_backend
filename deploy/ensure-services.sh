#!/usr/bin/env bash
#
# ensure-services.sh
# ------------------------------------------------------------------
# Self-healing watchdog for this host.
#
#  * Ensures every docker compose stack is up.
#      - already running  -> left untouched
#      - stopped / partial -> brought up (docker compose up -d)
#  * Ensures nginx is running.
#      - already active   -> left untouched
#      - failed/inactive  -> config is validated, then restarted
#      - if it still won't start, the port holders are logged so the
#        conflict (the recurring :8080 problem) is visible at a glance.
#
# Idempotent and safe to run repeatedly (cron / systemd timer / by hand).
# ------------------------------------------------------------------
set -u

LOG=/var/log/ensure-services.log
COMPOSE_STACKS=(
  /opt/grain_backend
  /opt/machine-config
  /crm-backend
)

# Standalone containers (started with plain `docker run`, not in a compose
# stack). Listed by container name; started only if present and stopped.
STANDALONE_CONTAINERS=(
  crm-frontend
)

ts()  { date '+%Y-%m-%d %H:%M:%S'; }
log() { echo "[$(ts)] $*" | tee -a "$LOG"; }

# --- 1. Docker compose stacks --------------------------------------
ensure_stacks() {
  for dir in "${COMPOSE_STACKS[@]}"; do
    if [ ! -d "$dir" ]; then
      log "SKIP  stack (dir missing): $dir"
      continue
    fi
    # --no-recreate: never touch a container that already exists (avoids
    # recreate-churn / name conflicts and protects running databases).
    # Missing or stopped containers are still created/started.
    if (cd "$dir" && docker compose up -d --no-recreate) >>"$LOG" 2>&1; then
      log "OK    stack up: $dir"
    else
      log "FAIL  stack up: $dir (see log above)"
    fi
  done
}

# --- 2. Standalone containers --------------------------------------
ensure_standalone() {
  for name in "${STANDALONE_CONTAINERS[@]}"; do
    state=$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null || echo missing)
    case "$state" in
      true)    log "OK    container running: $name" ;;
      false)
        if docker start "$name" >>"$LOG" 2>&1; then
          log "OK    container started: $name"
        else
          log "FAIL  container start: $name (see log above)"
        fi ;;
      *)       log "SKIP  container (not present): $name" ;;
    esac
  done
}

# --- 3. nginx ------------------------------------------------------
ensure_nginx() {
  if systemctl is-active --quiet nginx; then
    log "OK    nginx already active"
    return
  fi

  log "WARN  nginx not active -> attempting recovery"

  if ! nginx -t >>"$LOG" 2>&1; then
    log "FAIL  nginx config invalid -> NOT restarting; fix config (nginx -t)"
    return
  fi

  systemctl restart nginx >>"$LOG" 2>&1 || true

  if systemctl is-active --quiet nginx; then
    log "OK    nginx restarted"
  else
    log "FAIL  nginx still down -> likely a port conflict. Holders of 80/443/8080:"
    ss -tlnp 2>/dev/null | grep -E ':80 |:443 |:8080 ' | tee -a "$LOG"
    log "      (a docker container owning a port nginx wants will block startup;"
    log "       remove/disable the conflicting nginx listen directive.)"
  fi
}

log "===== ensure-services run start ====="
ensure_stacks
ensure_standalone
ensure_nginx
log "===== ensure-services run end ====="
