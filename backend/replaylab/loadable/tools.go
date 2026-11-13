//go:build tools

package main

// Keeps the bifrost-http release in go.mod, so the module graph (and every
// version the plugin shares with the gateway) is resolved against it. Never
// compiled.
import _ "github.com/maximhq/bifrost/transports/bifrost-http/lib"
