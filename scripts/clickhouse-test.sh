#!/usr/bin/env bash
# Runs the GH Archive query on a real ClickHouse server in Docker, with a
# small github_events table the test fills itself. Set
# STARTRUST_CLICKHOUSE_URL to use a server you already run instead.
set -euo pipefail

if [ -z "${STARTRUST_CLICKHOUSE_URL:-}" ]; then
    name=startrust-dev-clickhouse
    port=${STARTRUST_CLICKHOUSE_PORT:-18123}
    if ! docker ps --format '{{.Names}}' | grep -qx "$name"; then
        docker run -d --rm --name "$name" -p "127.0.0.1:$port:8123" \
            -e CLICKHOUSE_SKIP_USER_SETUP=1 clickhouse/clickhouse-server:24.8-alpine >/dev/null
        trap 'docker stop "$name" >/dev/null' EXIT
    fi
    for _ in $(seq 1 30); do
        curl -fsS "http://127.0.0.1:$port/ping" >/dev/null 2>&1 && break
        sleep 1
    done
    export STARTRUST_CLICKHOUSE_URL="http://127.0.0.1:$port/?user=default"
fi
go test ./internal/archive -run RealClickHouse -count=1 -v
