# ReplayLab glossary

**Loadable plugin**: Replay Lab built as a Go shared object (`loadable/`) that bifrost-http loads from a plugin entry in its config. This is one of three deployment modes. The other two are the SDK plugin, registered in code through the Go SDK, and the sidecar, `cmd/replaylab`'s capturing proxy. All three use the same core, `plugins/replaylab`.

**Target gateway version**: the one bifrost-http release a loadable plugin build is pinned to, for example `v1.6.11`. It's declared once, as the transports requirement in `loadable/go.mod`. The build artifact is named after it, the health response reports it as `target_gateway_version`, and the plugin only loads into a gateway built from that release with the same toolchain. See `docs/adr/0001-loadable-plugin-target-gateway.md`.
