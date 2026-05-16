#!/bin/sh
set -e

# Fix data directory permissions when running as root.
# Docker named volumes / host bind-mounts may be owned by root,
# preventing the non-root sub2api user from writing files.
if [ "$(id -u)" = "0" ]; then
    mkdir -p /app/data
    # Use || true to avoid failure on read-only mounted files (e.g. config.yaml:ro)
    chown -R sub2api:sub2api /app/data 2>/dev/null || true
    # Re-invoke this script as sub2api so the flag-detection below
    # also runs under the correct user.
    exec su-exec sub2api "$0" "$@"
fi

# Compatibility: if the first arg looks like a flag (e.g. --help),
# prepend the default binary so it behaves the same as the old
# ENTRYPOINT ["/app/sub2api"] style.
if [ "${1#-}" != "$1" ]; then
    set -- /app/sub2api "$@"
fi

is_truthy() {
    case "$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')" in
        1|true|yes|on) return 0 ;;
        *) return 1 ;;
    esac
}

should_start_local_redis() {
    if [ -n "${REDIS_URL:-}" ]; then
        return 1
    fi

    if [ -n "${LOCAL_REDIS_ENABLED:-}" ] && ! is_truthy "${LOCAL_REDIS_ENABLED}"; then
        return 1
    fi

    case "${REDIS_HOST:-}" in
        ""|localhost|127.0.0.1|::1) return 0 ;;
        *) return 1 ;;
    esac
}

start_local_redis() {
    REDIS_PORT="${REDIS_PORT:-6379}"
    REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
    REDIS_DB="${REDIS_DB:-0}"
    REDIS_ENABLE_TLS="${REDIS_ENABLE_TLS:-false}"
    LOCAL_REDIS_MAXMEMORY="${LOCAL_REDIS_MAXMEMORY:-128mb}"
    LOCAL_REDIS_MAXMEMORY_POLICY="${LOCAL_REDIS_MAXMEMORY_POLICY:-allkeys-lru}"
    LOCAL_REDIS_APPENDONLY="${LOCAL_REDIS_APPENDONLY:-no}"
    LOCAL_REDIS_SAVE="${LOCAL_REDIS_SAVE:-\"\"}"
    LOCAL_REDIS_DATABASES="${LOCAL_REDIS_DATABASES:-16}"

    export REDIS_HOST REDIS_PORT REDIS_DB REDIS_ENABLE_TLS

    redis_dir="/app/data/redis"
    redis_conf="${redis_dir}/redis.conf"
    mkdir -p "${redis_dir}"

    cat > "${redis_conf}" <<EOF
bind 127.0.0.1
protected-mode yes
port ${REDIS_PORT}
dir ${redis_dir}
pidfile ${redis_dir}/redis.pid
daemonize yes
databases ${LOCAL_REDIS_DATABASES}
appendonly ${LOCAL_REDIS_APPENDONLY}
save ${LOCAL_REDIS_SAVE}
maxmemory ${LOCAL_REDIS_MAXMEMORY}
maxmemory-policy ${LOCAL_REDIS_MAXMEMORY_POLICY}
tcp-keepalive 60
timeout 0
EOF

    if [ -n "${REDIS_PASSWORD:-}" ]; then
        printf '%s\n' "requirepass ${REDIS_PASSWORD}" >> "${redis_conf}"
    fi

    redis-server "${redis_conf}"

    attempts=0
    while [ "${attempts}" -lt 30 ]; do
        if [ -n "${REDIS_PASSWORD:-}" ]; then
            if redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" --pass "${REDIS_PASSWORD}" ping >/dev/null 2>&1; then
                echo "Embedded Redis started on ${REDIS_HOST}:${REDIS_PORT} (db=${REDIS_DB}, maxmemory=${LOCAL_REDIS_MAXMEMORY})"
                return 0
            fi
        elif redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" ping >/dev/null 2>&1; then
            echo "Embedded Redis started on ${REDIS_HOST}:${REDIS_PORT} (db=${REDIS_DB}, maxmemory=${LOCAL_REDIS_MAXMEMORY})"
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 1
    done

    echo "Failed to start embedded Redis on ${REDIS_HOST}:${REDIS_PORT}" >&2
    exit 1
}

if should_start_local_redis; then
    start_local_redis
fi

exec "$@"
