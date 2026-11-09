# ADR-0001: Target gateway for the loadable plugin

Status: accepted (2026-11-09). Verified: the smoke test passes against bifrost-http v1.6.11 built from source on Linux.

## Context

bifrost-http loads native Go plugins as shared objects with Go's `plugin` package. A shared object only loads into a gateway built with the same Go toolchain, the same build flags that affect package hashes (`-trimpath`, tags), and the same version of every module the two binaries share. So the **loadable plugin** has to be built for one named bifrost-http release: its **target gateway version**.

The newest bifrost-http release is transports v1.6.11. Reading its source showed:

- It requires Bifrost core **v1.7.10** and framework **v1.5.10** (not v1.12.1, as the spec assumed), on Go **1.26.5**. It has no `replace` directives, so there are none to repeat. In particular it uses upstream `mark3labs/mcp-go` v0.43.2, not the maximhq fork that core v1.15.1 needs.
- The framework v1.5.10 loader looks up `Init(config any) error` (optional), `GetName` and `Cleanup` (required), and these optional hooks by name: `HTTPTransportPreHook`, `HTTPTransportPostHook`, `HTTPTransportStreamChunkHook`, `PreRequestHook`, `PreLLMHook` (or legacy `PreHook`), `PostLLMHook` (or `PostHook`), the MCP hooks and `Inject`. It has no pre-auth or response-headers hook.
- **The official image can't load any plugin.** `transports/Dockerfile` links the binary with `-extldflags '-static'`, and a static binary can't `dlopen` a shared object.
- The gateway runs plugin transport hooks only on inference routes. A request for an unknown path such as `/replaylab/health` goes to the router's 404 handler, or to the UI's catch-all route, which has no middleware. Neither reaches the plugin.
- If a plugin's `Init` fails, the gateway logs `failed to load plugin <name>: ...`, marks the plugin as errored, and keeps serving. Startup doesn't abort.

Replay Lab's shared core (`plugins/replaylab`) builds against core v1.15.1 on Go 1.27.3.

## Decision

**Target bifrost-http v1.6.11 (the latest release), built from that release's source, with dynamic linking.**

- The gateway is the release's own source from the Go module proxy, built with the release's own command minus `-extldflags '-static'` and `-s -w`. `loadable/Dockerfile` builds it in `golang:1.26.5-alpine3.23`, the release's builder image. A source build of the same release is the only kind of v1.6.11 that can load plugins, and it keeps the release's exact module versions.
- `loadable/` is its own module. It requires `transports v1.6.11`, which is the single place the target gateway version is declared.
- **Versions are pinned by selection, not by `replace`.** The first version pinned shared modules with `replace` directives. It passed a module-level check, but the real gateway rejected the plugin with `plugin was built with a different version of package github.com/bytedance/sonic/loader/internal/rt`. Under `-trimpath`, a replaced module's file paths still record the *required* version (`loader@v0.5.2`, not the `v0.5.1` actually built), so the package fingerprint differs from the gateway's. So the build now gives the shared core's copy a `go.mod` with no requirements (`prepare-core.sh`). `loadable/go.mod` lists what the core imports, and the release's own requirements decide every shared version.
- Replay Lab's one dependency that the gateway doesn't have, `modernc.org/sqlite`, is held at **v1.53.0** in the loadable build. That's the newest release whose own dependencies (`x/sys` and others) don't overtake the gateway's. The SDK build keeps v1.61.1.
- `check-pins.sh` takes the modules that supply packages linked into the plugin and compares their versions with the gateway's build list. It fails the build on any difference, and on any module replaced with a version.
- **The shared core stays compatible with both core versions.** Only one thing in it doesn't compile against v1.7.10: the no-op `HTTPTransportResponseHeadersHook`, which uses `HTTPResponseMetadata`. That type was added later. The hook and the `HTTPTransportPlugin` assertion moved to `bifrost_current_core.go`, which the loadable build excludes with `-tags replaylab_core_v1_7`. Nothing else in the core is version-specific.
- The core's `go.mod` keeps `go 1.27.3` and its own requirements for SDK users. `prepare-core.sh` also sets the copy's `go` line to the gateway's Go version, because a dependency declaring a newer Go can't be built on Go 1.26.5. The core compiles on both.
- The entry point exports `Init`, `GetName`, `Cleanup`, `HTTPTransportPreHook`, `PreLLMHook` and `PostLLMHook`. Each one delegates to a single Replay Lab instance. It doesn't export the transport post-hook or the stream-chunk hook, because Replay Lab's versions of those are no-ops and the loader treats a missing hook as a pass-through.

Two consequences follow from the gateway's behaviour:

- **API:** because v1.6.11 never routes `/replaylab/*` through plugin hooks, in this mode the API is served on the plugin's own listener. It defaults to `:8086`, and `"api_listen_addr": ""` turns it off. `HTTPTransportPreHook` is still exported, so the API is also served on the gateway port by any release that routes such paths through plugin hooks. It still marks replays and copies tag and tenant headers on inference routes.
- **Bad config:** "fails at startup" means the plugin doesn't load, and the gateway logs the error. The error names the plugin and its target gateway version, and the Replay Lab API never comes up. The gateway itself keeps running, because that's how bifrost-http treats every failing plugin.

## Alternatives considered

- **Drop the shared core to core v1.7.10.** SDK users would lose everything newer, so this was rejected.
- **Target an older release whose core is closer to v1.15.1.** No released bifrost-http uses a newer core than v1.6.11 does, and any older release would have the same static-image problem.
- **Copy the core into the loadable module at build time.** That makes two sources of truth, so it was rejected in favour of one build tag.
- **WASM plugin.** That would avoid the lock-step entirely, but it's out of scope (see the spec).

## Upgrading the target gateway

1. In `loadable/go.mod`, change the transports requirement to the new release, and set the `go` directive to the release's Go version.
2. In `loadable/Dockerfile`, set `GO_VERSION` and `ALPINE_VERSION` to the new release's builder image. The build stops if `GO_VERSION` differs from the `go` directive.
3. Re-check the loader's symbol names and signatures, and whether the new core still needs the `replaylab_core_v1_7` tag.
4. Rebuild. `check-pins.sh` names any shared module whose version differs. Fix it by adjusting `loadable/go.mod`'s requirements so selection lands on the gateway's version, for example by moving `modernc.org/sqlite` to the newest release that fits. Never use `replace` for this.
5. Run `make loadable-smoke`. CI's `loadable` job runs it on every push too.

Nothing else names the version: the `.so` name, the health response and the error messages are stamped from `loadable/go.mod` at build time, and the demo config points at `/plugins/replaylab.so`, which links to the versioned file.
