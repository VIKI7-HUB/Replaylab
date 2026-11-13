#!/bin/sh
# Starts bifrost-http with Replay Lab loaded, then the router on Render's $PORT.
set -eu

# The gateway routes to every provider whose API key is set, so changing
# providers is a change to the service's environment, not to the image.
providers=""
add_provider() { # <bifrost provider name> <environment variable holding its key>
	eval "key=\${$2:-}"
	[ -n "$key" ] || return 0
	[ -z "$providers" ] || providers="$providers, "
	providers="$providers\"$1\": {\"keys\": [{\"name\": \"$1\", \"value\": \"env.$2\", \"models\": [\"*\"], \"weight\": 1}]}"
}
add_provider gemini GEMINI_API_KEY
add_provider groq GROQ_API_KEY
if [ -z "$providers" ]; then
	echo "replaylab: no provider key set; set GEMINI_API_KEY (or GROQ_API_KEY)" >&2
	exit 1
fi
sed "s|__PROVIDERS__|$providers|" /render/config.template.json > "$APP_DIR/config.json"

/app/bifrost-http -app-dir "$APP_DIR" -port 8080 -host 127.0.0.1 &
exec caddy run --config /render/Caddyfile --adapter caddyfile
