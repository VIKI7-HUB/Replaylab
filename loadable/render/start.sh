#!/bin/sh
# Starts bifrost-http with Replay Lab loaded, then the router on Render's $PORT.
set -eu
cp /render/config.json "$APP_DIR/config.json"
/app/bifrost-http -app-dir "$APP_DIR" -port 8080 -host 127.0.0.1 &
exec caddy run --config /render/Caddyfile --adapter caddyfile
