# Builds the target bifrost-http release and the Replay Lab loadable plugin
# with the same Go toolchain, build flags and module versions, then (in the
# "smoke" stage) runs the gateway smoke test against them.
#
# Run from bifrost-backend/ (the build context is the whole Go module):
#
#   docker build -f loadable/Dockerfile --target smoke .            # build + smoke test
#   docker build -f loadable/Dockerfile --target plugin -o dist .   # just the .so, into dist/
#   docker build -f loadable/Dockerfile --target gateway -t replaylab-gateway .
#   docker compose -f loadable/compose.yml up --build               # gateway demo
#
# In the gateway image, /plugins/replaylab.so links to the versioned shared object,
# so gateway configs needn't repeat the version.
#
# The target gateway version comes from loadable/go.mod (the transports
# requirement). GO_VERSION and ALPINE_VERSION must match that release's own
# Dockerfile (transports/Dockerfile in the Bifrost repo).
#
# Why the gateway is built here rather than taken from the official image: the
# official image links bifrost-http with -extldflags '-static', and a static
# binary cannot load shared objects. See
# docs/adr/0001-loadable-plugin-target-gateway.md.

ARG GO_VERSION=1.26.5
ARG ALPINE_VERSION=3.23

# --- gateway + plugin --------------------------------------------------------
FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS build
ARG GO_VERSION
RUN apk add --no-cache gcc musl-dev sqlite-dev binutils binutils-gold
ENV CGO_ENABLED=1 GOOS=linux GOTOOLCHAIN=local GOWORK=off
WORKDIR /src

COPY go.mod ./
COPY loadable/go.mod loadable/go.sum loadable/prepare-core.sh loadable/
# loadable/go.mod's go directive is the target release's Go version. This
# image's toolchain must be exactly that, or the plugin won't load.
RUN want=$(awk '$1 == "go" { print $2; exit }' loadable/go.mod) \
 && if [ "$want" != "${GO_VERSION}" ]; then \
      echo "GO_VERSION=${GO_VERSION} but loadable/go.mod says go $want: set GO_VERSION (and ALPINE_VERSION) to the target release's builder image" >&2; exit 1; \
    fi
# Give the shared core a requirement-free go.mod on the gateway's Go version,
# so the target release decides every shared module version (see the script).
RUN sh loadable/prepare-core.sh /src ${GO_VERSION} \
 && awk '$1 == "github.com/maximhq/bifrost/transports" { print $2; exit }' loadable/go.mod > /gateway-version \
 && test -s /gateway-version \
 && echo "target gateway: bifrost-http $(cat /gateway-version)"

# The target release's source, from the Go module proxy. The module zip has no
# UI bundle, so embed a placeholder page (the UI is out of scope here).
RUN V=$(cat /gateway-version) \
 && dir=$(go mod download -json "github.com/maximhq/bifrost/transports@$V" | sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p') \
 && cp -r "$dir" /gateway && chmod -R u+w /gateway \
 && mkdir -p /gateway/bifrost-http/ui \
 && printf '<!doctype html><title>bifrost-http %s</title><p>UI not bundled in this build.</p>\n' "$V" > /gateway/bifrost-http/ui/index.html \
 && (cd /gateway && go mod download) \
 && (cd /src/loadable && go mod download)

COPY plugins/ plugins/
COPY loadable/ loadable/

# Fail early, with the module names, if any shared module differs.
RUN sh loadable/check-pins.sh /gateway /src/loadable "sqlite_static replaylab_core_v1_7"

# The release's own build command (transports/Dockerfile), minus
# "-extldflags '-static'" and "-s -w": the gateway must be dynamically linked
# to load plugins.
RUN cd /gateway && go build -a -trimpath -tags "sqlite_static" \
      -ldflags "-X main.Version=v$(cat /gateway-version)" \
      -o /out/bifrost-http ./bifrost-http

# The plugin: same toolchain, -trimpath and tags as the gateway, plus the tag
# that drops core-version-specific code from the shared core.
RUN V=$(cat /gateway-version) && cd /src/loadable \
 && go build -buildmode=plugin -trimpath -tags "sqlite_static replaylab_core_v1_7" \
      -ldflags "-X main.targetGatewayVersion=$V -X github.com/maximhq/bifrost/plugins/replaylab/loadable.targetGatewayVersion=$V" \
      -o "/out/plugins/replaylab-bifrost-http-$V.so" . \
 && go build -o /out/smoketest ./smoketest \
 && cp /gateway-version /out/plugins/TARGET_GATEWAY_VERSION

# --- mock provider (the shared module's own toolchain) -----------------------
FROM golang:1.27-alpine AS mock
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal/ internal/
COPY cmd/mockprovider/ cmd/mockprovider/
RUN CGO_ENABLED=0 go build -trimpath -o /out/mockprovider ./cmd/mockprovider
COPY demo/generate_traffic/ demo/generate_traffic/
RUN CGO_ENABLED=0 go build -trimpath -o /out/generate_traffic ./demo/generate_traffic

# --- demo traffic for loadable/compose.yml ------------------------------------
FROM alpine:${ALPINE_VERSION} AS seed
COPY --from=mock /out/generate_traffic /app/generate_traffic
COPY demo/sample_prompts.json /app/sample_prompts.json
COPY loadable/demo/seed.sh /app/seed.sh
ENTRYPOINT ["/bin/sh", "/app/seed.sh"]

# --- just the shared object --------------------------------------------------
FROM scratch AS plugin
COPY --from=build /out/plugins/ /

# --- the target gateway with the plugin available ----------------------------
FROM alpine:${ALPINE_VERSION} AS gateway
RUN apk add --no-cache musl libgcc ca-certificates zlib \
 && adduser -D -s /bin/sh appuser && mkdir -p /app/data && chown -R appuser /app
COPY --from=build /out/bifrost-http /app/bifrost-http
COPY --from=build /out/plugins/ /plugins/
# A version-free name for gateway configs. Only in the image: the "plugin"
# export has no symlink, since Windows can't create one without admin rights.
RUN ln -s "replaylab-bifrost-http-$(cat /plugins/TARGET_GATEWAY_VERSION).so" /plugins/replaylab.so
ENV APP_DIR=/app/data APP_PORT=8080
USER appuser
EXPOSE 8080 8086
ENTRYPOINT ["/bin/sh", "-c", "exec /app/bifrost-http -app-dir \"$APP_DIR\" -port \"$APP_PORT\" -host 0.0.0.0 \"$@\"", "--"]

# --- smoke test: the build fails if the plugin doesn't work in the gateway ----
FROM gateway AS smoke
COPY --from=build /out/smoketest /app/smoketest
COPY --from=mock /out/mockprovider /app/mockprovider
RUN V=$(cat /plugins/TARGET_GATEWAY_VERSION) \
 && /app/smoketest -gateway /app/bifrost-http -mock /app/mockprovider \
      -plugin "/plugins/replaylab-bifrost-http-$V.so" -version "$V" -workdir /tmp/smoke

# --- Render: the gateway plus a router on the one public port ----------------
# Render builds a Dockerfile's last stage and can't pick a target, so this
# stage stays last. Every other use names its target explicitly.
#
#   gateway   bifrost-http on 127.0.0.1:8080, Replay Lab's API on 127.0.0.1:8086
#   router    Caddy on $PORT: /replaylab/* to the API, everything else to the gateway
#
# Needs a provider key: GEMINI_API_KEY, GROQ_API_KEY or both (see
# loadable/render/start.sh, which writes the gateway config from them).
FROM gateway AS render
USER root
RUN apk add --no-cache caddy
COPY loadable/render/ /render/
RUN sed -i 's/\r$//' /render/start.sh && chmod +x /render/start.sh
USER appuser
ENTRYPOINT ["/render/start.sh"]
