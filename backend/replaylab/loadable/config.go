package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

// defaultAPIListenAddr is where the Replay Lab API listens when the config
// block doesn't say. bifrost-http v1.6.11 runs plugin transport hooks only on
// its inference routes, so /replaylab/* on the gateway port never reaches the
// plugin; the dedicated listener is how the API is reached. Set
// "api_listen_addr": "" to turn it off.
const defaultAPIListenAddr = ":8086"

// pluginConfig is the plugin entry's "config" block. Keys are the Replay Lab
// config's JSON names; any other key is rejected.
type pluginConfig struct {
	replaylab.Config
	// ReplayAuthToken is sent as the bearer token on replays, for gateways that
	// require a virtual key. Replay Lab's Config never serialises it, so it is
	// declared here.
	ReplayAuthToken string `json:"replay_auth_token"`
}

// decodeConfig maps the plugin entry's config value onto Replay Lab config.
// Omitted keys keep their defaults; an absent or empty block yields the
// defaults. The gateway hands over whatever it decoded: usually a JSON object
// as map[string]any, but a JSON string or raw bytes are accepted too.
func decodeConfig(raw any) (replaylab.Config, error) {
	data, err := configJSON(raw)
	if err != nil {
		return replaylab.Config{}, err
	}

	cfg := pluginConfig{Config: gatewayDefaults()}
	if len(data) > 0 {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			return replaylab.Config{}, fmt.Errorf("invalid plugin config: %w", err)
		}
		if dec.More() {
			return replaylab.Config{}, errors.New("invalid plugin config: trailing data after the config object")
		}
	}
	out := cfg.Config
	out.ReplayAuthToken = cfg.ReplayAuthToken

	if out.DBPath == "" {
		return replaylab.Config{}, errors.New("invalid plugin config: db_path must not be empty")
	}
	if out.GatewayURL == "" {
		return replaylab.Config{}, errors.New("invalid plugin config: gateway_url must not be empty")
	}
	if out.MaxQueueSize < 0 || out.WorkerCount < 0 {
		return replaylab.Config{}, errors.New("invalid plugin config: max_queue_size and worker_count must not be negative")
	}
	if err := out.Validate(); err != nil {
		return replaylab.Config{}, fmt.Errorf("invalid plugin config: %w", err)
	}
	return out, nil
}

// configJSON normalises the loader's config value to JSON object bytes, or
// nil when there is no config.
func configJSON(raw any) ([]byte, error) {
	var data []byte
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case string:
		data = []byte(v)
	case []byte:
		data = v
	case json.RawMessage:
		data = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("invalid plugin config: %w", err)
		}
		data = b
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil, nil
	}
	if data[0] != '{' {
		return nil, errors.New("invalid plugin config: expected a JSON object")
	}
	return data, nil
}

// gatewayDefaults are Replay Lab's defaults adjusted for running inside the
// gateway: storage goes in the gateway's data directory, replays go back to
// the gateway itself, and the API gets its own port.
func gatewayDefaults() replaylab.Config {
	cfg := replaylab.DefaultConfig()
	if dir := os.Getenv("APP_DIR"); dir != "" {
		cfg.DBPath = filepath.Join(dir, "replaylab.db")
	}
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080" // bifrost-http's default port
	}
	cfg.GatewayURL = "http://127.0.0.1:" + port
	cfg.APIListenAddr = defaultAPIListenAddr
	return cfg
}
