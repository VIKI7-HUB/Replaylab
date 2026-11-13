# Replay Lab as a bifrost-http plugin

This is the **loadable plugin**: Replay Lab built as a shared object that a running bifrost-http gateway loads from its config. The gateway then captures chat traffic through Replay Lab's hooks, serves the Replay Lab API, and receives Replay Lab's replays, so your keys, routing and governance apply to them. Replays are never captured as new traffic.

**Target gateway version: bifrost-http v1.6.11** (Bifrost core v1.7.10, Go 1.26.5). A build loads only into that release. See [ADR-0001](../docs/adr/0001-loadable-plugin-target-gateway.md) for why.

## Try it in one command

From `bifrost-backend/`, with Docker (Docker Desktop on Windows and macOS):

```bash
docker compose -f loadable/compose.yml up --build
```

This builds bifrost-http v1.6.11 from source and the plugin with it, starts the gateway with Replay Lab enabled ([demo/config.json](demo/config.json)) and the deterministic mock provider as its upstream, so no API keys are needed. A `seed` job then sends 120 chat requests through the gateway, waits until they're captured, starts an experiment (`gpt-4o` against `gpt-4o-mini` and `meta-llama-3.1-8b`) whose replays also go through the gateway, and prints the URLs to open. The first build takes a few minutes, because it compiles the gateway.

| URL | What |
| :--- | :--- |
| `http://localhost:8080/v1/chat/completions` | The gateway. Send `"model": "openai/gpt-4o"` to add captures |
| `http://localhost:8086/replaylab/health` | Replay Lab, including `target_gateway_version` |
| `http://localhost:8086/replaylab/experiments` | Experiments and their verdicts |

`make gateway-demo` runs the same command. `docker compose -f loadable/compose.yml down -v` removes it, including the captures.

## Requirements

- **Linux or macOS.** Go doesn't support shared-object plugins on Windows. On Windows, use the container build (the demo above or `make loadable`, through Docker Desktop or WSL), or run Replay Lab as a sidecar (`cmd/replaylab`; see the main [README](../README.md#deployment-modes)).
- **A bifrost-http v1.6.11 that is dynamically linked.** The official `maximhq/bifrost` image is linked with `-static` and **can't load any plugin**. Use the gateway this Dockerfile builds (the `gateway` target), or build v1.6.11 yourself without `-extldflags '-static'`, on Go 1.26.5 with `-trimpath` and `-tags sqlite_static`.

## Build

From `bifrost-backend/`:

```bash
make loadable          # dist/replaylab-bifrost-http-v1.6.11.so
make loadable-smoke    # build the gateway + plugin and run the gateway smoke test
```

Both run `docker build -f loadable/Dockerfile`. Its other target, `gateway`, is an image of bifrost-http v1.6.11 with the plugin at `/plugins/replaylab-bifrost-http-v1.6.11.so`. `/plugins/replaylab.so` links to it, so a config that uses that path doesn't change when the target gateway version does.

CI ([../.github/workflows/ci.yml](../.github/workflows/ci.yml), job `loadable`) builds the shared object and runs the gateway smoke test on Linux on every push, and keeps the `.so` as a build artifact.

## Configure

Add one entry to the gateway's `config.json` `plugins` list:

```json
{
  "name": "replaylab",
  "path": "/plugins/replaylab-bifrost-http-v1.6.11.so",
  "enabled": true,
  "config": {}
}
```

An empty `config` works. Every key is optional. Unknown keys are rejected. This entry sets every supported key, to its default in the gateway (it's the entry the demo uses, in [demo/config.json](demo/config.json)):

```json
{
  "name": "replaylab",
  "path": "/plugins/replaylab.so",
  "enabled": true,
  "config": {
    "db_path": "/app/data/replaylab.db",
    "gateway_url": "http://127.0.0.1:8080",
    "replay_auth_token": "",
    "api_listen_addr": ":8086",
    "enabled": true,
    "sampling_rate": 1.0,
    "opt_in_keys": [],
    "opt_out_keys": [],
    "redact_pii": true,
    "store_hashes_only": false,
    "retention_days": 30,
    "default_budget_cap": 50,
    "max_stream_capture_bytes": 262144,
    "max_queue_size": 10000,
    "worker_count": 4
  }
}
```

| Key | Default | Meaning |
| :--- | :--- | :--- |
| `db_path` | `$APP_DIR/replaylab.db` (else `./replaylab.db`) | SQLite file for captures and experiments |
| `gateway_url` | `http://127.0.0.1:$APP_PORT` (port 8080) | Where replays are sent; the gateway itself by default |
| `replay_auth_token` | none | Bearer token for replays, if the gateway requires a virtual key |
| `api_listen_addr` | `:8086` | Address the Replay Lab API listens on; `""` turns it off |
| `enabled` | `true` | Capture on or off (the API stays up) |
| `sampling_rate` | `1.0` | Fraction of requests to capture, 0 to 1 |
| `opt_in_keys` / `opt_out_keys` | none | Capture only these virtual keys, or skip these |
| `redact_pii` | `true` | Redact emails, phone numbers, card numbers and API keys |
| `store_hashes_only` | `false` | Store hashes instead of message and response text |
| `retention_days` | `30` | Delete captures older than this; `0` keeps them |
| `default_budget_cap` | `50` | Default experiment budget cap, in dollars |
| `max_stream_capture_bytes` | `262144` | Cap on the content kept per streamed capture |
| `max_queue_size` / `worker_count` | `10000` / `4` | Capture queue size and writer count |

The runtime keys can still be changed later through `PUT /replaylab/config`.

**The API is on its own port (`:8086`), not the gateway's.** bifrost-http v1.6.11 runs plugin hooks only on its inference routes, so `/replaylab/*` on the gateway port never reaches the plugin. The dedicated port also gives live SSE progress.

**Invalid config:** the plugin doesn't load and the gateway logs, for example:

```
failed to load plugin replaylab: failed to load custom plugin: plugin Init failed: replaylab (built for bifrost-http v1.6.11): invalid plugin config: sampling_rate must be between 0 and 1
```

bifrost-http keeps running without the plugin, which is how it treats every plugin that fails to load. The plugin shows as errored in `GET /api/plugins`.

**Check the version** with `GET :8086/replaylab/health`. The response includes `"target_gateway_version": "v1.6.11"`.

## Loader errors and what they mean

| Gateway log contains | Meaning | Fix |
| :--- | :--- | :--- |
| `plugin was built with a different version of package ...` | The gateway isn't the target release, was built with a different Go version, or was built with different flags (`-trimpath`) | **Rebuild the plugin for your gateway version**, or run the target gateway (v1.6.11) |
| `Dynamic loading not supported` / `plugin: not implemented` | The gateway binary is statically linked (the official image), or is running on Windows | Use a dynamically linked gateway, such as the `gateway` image target |
| `failed to cast ... to expected signature` | The plugin was built against another core's hook types | Rebuild for your gateway version |
| `plugin Init failed: replaylab (built for bifrost-http ...)` | The plugin loaded, but its config block is invalid | Fix the key named in the message |

## Changing the target gateway version

The target gateway version is declared in one place: the `github.com/maximhq/bifrost/transports` requirement in `loadable/go.mod`. The Makefile, the Dockerfile, the `.so` file name, the health response and the error messages all take it from there. Upgrading is one reviewable change:

1. **Change the version.** In `loadable/go.mod`, set the transports requirement to the new release and the `go` directive to that release's Go version. Set `GO_VERSION` and `ALPINE_VERSION` in `loadable/Dockerfile` to the release's builder image; the build fails with a clear message if `GO_VERSION` and the `go` directive disagree.
2. **Rebuild:** `make loadable`. If `check-pins.sh` names a module whose version differs from the gateway's, adjust `loadable/go.mod`'s requirements (never with `replace`).
3. **Rerun the smoke test:** `make loadable-smoke`, or push and let CI run it.

The details, including what to re-check in the new loader, are in [ADR-0001](../docs/adr/0001-loadable-plugin-target-gateway.md#upgrading-the-target-gateway). Gateway configs that use `/plugins/replaylab.so` need no change.

## Layout

- `main.go`: the exported loader symbols. Each one delegates to one Replay Lab instance. No capture or replay logic lives here.
- `config.go`: maps the `config` block onto Replay Lab's config.
- `go.mod`: the target release, and what the shared core imports.
- `prepare-core.sh`: gives the shared core's copy a requirement-free `go.mod`, so the release's requirements decide every shared version.
- `check-pins.sh`: checks that every module linked into the plugin matches the gateway's version.
- `Dockerfile`: builds the gateway and the plugin, runs the smoke test, and builds the demo's `seed` image.
- `compose.yml`, `demo/`: the one-command gateway demo, its gateway config and its seed script.
- `smoketest/`: starts the real gateway and the mock provider, and asserts over HTTP only.
