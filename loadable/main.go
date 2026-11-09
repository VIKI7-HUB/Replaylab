// Command loadable is Replay Lab's loadable plugin: a shared object that
// bifrost-http loads from a plugin entry in its config.
//
//	{ "name": "replaylab", "path": "/plugins/replaylab-bifrost-http-v1.6.11.so",
//	  "enabled": true, "config": { "sampling_rate": 0.2 } }
//
// Build it with -buildmode=plugin and -tags replaylab_core_v1_7, on the target
// gateway's exact Go toolchain (see Dockerfile and Makefile). The exported
// functions below are the symbols the gateway's shared-object loader looks up
// by name; each delegates to one Replay Lab instance created in Init. No
// capture or replay logic lives here.
package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// targetGatewayVersion is the bifrost-http release this build is pinned to.
// The build stamps it from go.mod's transports requirement with
// -ldflags "-X main.targetGatewayVersion=...".
var targetGatewayVersion = "unknown"

var (
	mu  sync.RWMutex
	lab *replaylab.Plugin
)

func current() *replaylab.Plugin {
	mu.RLock()
	defer mu.RUnlock()
	return lab
}

// Init is called once by the gateway's loader with the plugin entry's config
// block. An error here keeps the plugin from loading, and the gateway logs it.
func Init(config any) error {
	cfg, err := decodeConfig(config)
	if err != nil {
		return fmt.Errorf("replaylab (built for bifrost-http %s): %w", targetGatewayVersion, err)
	}

	mu.Lock()
	defer mu.Unlock()
	// The gateway re-runs Init when a plugin is reloaded. Release the old
	// instance first so its storage and API port are free.
	if lab != nil {
		_ = lab.Cleanup()
		lab = nil
	}
	p, err := replaylab.NewPlugin(cfg)
	if err != nil {
		return fmt.Errorf("replaylab (built for bifrost-http %s): %w", targetGatewayVersion, err)
	}
	p.GetAPIHandler().SetTargetGatewayVersion(targetGatewayVersion)
	lab = p

	api := "not served on its own port"
	if cfg.APIListenAddr != "" {
		api = "on " + cfg.APIListenAddr
	}
	log.Printf("replaylab: loaded into bifrost-http (built for %s); db=%s, replays via %s, API %s",
		targetGatewayVersion, cfg.DBPath, cfg.GatewayURL, api)
	return nil
}

// GetName returns the plugin's name.
func GetName() string {
	return replaylab.PluginName
}

// Cleanup drains queued captures and closes storage.
func Cleanup() error {
	mu.Lock()
	defer mu.Unlock()
	if lab == nil {
		return nil
	}
	err := lab.Cleanup()
	lab = nil
	return err
}

// HTTPTransportPreHook serves /replaylab/* when the gateway routes such a
// request through plugin hooks, and copies Replay Lab headers (replay marker,
// tag, tenant) onto the request context.
func HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	p := current()
	if p == nil {
		return nil, nil
	}
	return p.HTTPTransportPreHook(ctx, req)
}

// PreLLMHook records chat requests.
func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	p := current()
	if p == nil {
		return req, nil, nil
	}
	return p.PreLLMHook(ctx, req)
}

// PostLLMHook pairs responses (and stream chunks) with recorded requests.
func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	p := current()
	if p == nil {
		return resp, bifrostErr, nil
	}
	return p.PostLLMHook(ctx, resp, bifrostErr)
}

func main() {}
