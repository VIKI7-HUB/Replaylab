// Command smoketest drives a real bifrost-http gateway with the Replay Lab
// loadable plugin configured, over HTTP only. It starts the deterministic mock
// provider as the gateway's upstream, then checks:
//
//   - an empty config block loads with safe defaults, and health reports the
//     target gateway version
//   - chat traffic through the gateway is captured, tagged and redacted
//   - an experiment replays through the gateway and reaches the expected
//     verdicts, and its replays are not captured
//   - captures survive a gateway restart
//   - explicit config keys take effect
//   - invalid config keeps the plugin from loading, with a clear message
//
// It runs where shared-object plugins work (Linux, in the loadable/Dockerfile
// "smoke" stage). It never looks inside the plugin.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	gatewayBin = flag.String("gateway", "/app/bifrost-http", "bifrost-http binary")
	pluginPath = flag.String("plugin", "", "Replay Lab shared object")
	mockBin    = flag.String("mock", "/app/mockprovider", "mock provider binary")
	version    = flag.String("version", "", "target gateway version the plugin must report")
	workDir    = flag.String("workdir", "", "scratch directory (default: a temp dir)")
)

const (
	gatewayPort = "8080"
	mockPort    = "8081"
	apiPort     = "8086" // the plugin's default API port
	altAPIPort  = "8087"

	gatewayURL = "http://127.0.0.1:" + gatewayPort
	apiURL     = "http://127.0.0.1:" + apiPort
	altAPIURL  = "http://127.0.0.1:" + altAPIPort

	email = "jane.doe@example.com"
	tag   = "smoke"
)

var prompts = []string{
	"Extract the structured JSON object containing name, email, and company from: 'Bo, bo@x.io, Acme'",
	"Explain why atomic CPU operations avoid race conditions in concurrent code. Reply to " + email + ".",
	"Summarize the advantages of micro-frontends versus monolithic single page apps.",
}

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime)
	if *pluginPath == "" || *version == "" {
		log.Fatal("-plugin and -version are required")
	}
	dir := *workDir
	if dir == "" {
		d, err := os.MkdirTemp("", "replaylab-smoke-")
		must(err)
		dir = d
	}

	mock := start("mock", []string{"PORT=" + mockPort, "MOCK_LATENCY_SCALE=0.25"}, *mockBin)
	defer mock.stop()
	waitHTTP("mock provider", "http://127.0.0.1:"+mockPort+"/health")

	step("empty config: defaults, capture, experiment, replays not captured", func() { mainScenario(dir) })
	step("explicit config keys take effect", func() { explicitConfig(dir) })
	step("invalid value: plugin not loaded, clear message", func() {
		invalidConfig(dir, map[string]any{"sampling_rate": 2}, "sampling_rate must be between 0 and 1")
	})
	step("unknown key: plugin not loaded, clear message", func() {
		invalidConfig(dir, map[string]any{"sampling_rat": 0.5}, `unknown field "sampling_rat"`)
	})
	log.Printf("PASS: Replay Lab loads and works in bifrost-http %s", *version)
}

func mainScenario(root string) {
	appDir := filepath.Join(root, "main")
	gw := startGateway(appDir, map[string]any{})

	health := waitHealth(apiURL)
	if got := health["target_gateway_version"]; got != *version {
		fail("health target_gateway_version = %v, want %s", got, *version)
	}
	if _, err := os.Stat(filepath.Join(appDir, "replaylab.db")); err != nil {
		fail("default db_path should be in the gateway's APP_DIR: %v", err)
	}

	const n = 24
	for i := 0; i < n; i++ {
		chat("gpt-4o", prompts[i%len(prompts)], map[string]string{"X-Replaylab-Tag": tag})
	}
	waitCaptures(apiURL, n)

	var captures []map[string]any
	getJSON(apiURL+"/replaylab/captures?limit=100", &captures)
	if len(captures) != n {
		fail("listed %d captures, want %d", len(captures), n)
	}
	for _, c := range captures {
		if c["model"] != "gpt-4o" || c["tag"] != tag {
			fail("capture has model=%v tag=%v, want gpt-4o/%s", c["model"], c["tag"], tag)
		}
		if msgs, _ := c["messages_json"].(string); strings.Contains(msgs, email) {
			fail("capture %v was not redacted: %s", c["id"], msgs)
		}
	}
	log.Printf("  %d captures, tagged and redacted", n)

	var exp map[string]any
	postJSON(apiURL+"/replaylab/experiments", map[string]any{
		"name":             "gateway smoke",
		"original_model":   "gpt-4o",
		"candidate_models": []string{"openai/gpt-4o-mini", "openai/meta-llama-3.1-8b"},
		"sample_size":      n,
	}, &exp)
	id, _ := exp["id"].(string)
	if id == "" {
		fail("experiment has no id: %v", exp)
	}
	final := waitExperiment(id)
	if final["status"] != "completed" {
		fail("experiment status = %v, want completed (stop reason %v)", final["status"], final["stop_reason"])
	}
	want := map[string]string{
		"openai/gpt-4o-mini":       "SAFE_TO_SWITCH",
		"openai/meta-llama-3.1-8b": "DO_NOT_SWITCH",
	}
	recs, _ := final["recommendations"].([]any)
	if len(recs) != len(want) {
		fail("got %d recommendations, want %d", len(recs), len(want))
	}
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		model, _ := rec["candidate_model"].(string)
		if rec["verdict"] != want[model] {
			fail("%s verdict = %v, want %s", model, rec["verdict"], want[model])
		}
	}
	log.Printf("  experiment %s completed with the expected verdicts", id)

	time.Sleep(time.Second) // let any (wrongly) captured replay reach storage
	if got := storedCaptures(apiURL); got != n {
		fail("replays were captured: %d captures after the experiment, want %d", got, n)
	}
	log.Printf("  replays were not captured")

	gw.stop()
	gw = startGateway(appDir, map[string]any{})
	defer gw.stop()
	waitHealth(apiURL)
	if got := storedCaptures(apiURL); got != n {
		fail("after restart: %d captures, want %d", got, n)
	}
	var again map[string]any
	getJSON(apiURL+"/replaylab/experiments/"+id, &again)
	if again["status"] != "completed" {
		fail("after restart: experiment status = %v", again["status"])
	}
	log.Printf("  captures and experiment survived a gateway restart")
}

func explicitConfig(root string) {
	appDir := filepath.Join(root, "explicit")
	dbPath := filepath.Join(appDir, "custom", "lab.db")
	must(os.MkdirAll(filepath.Dir(dbPath), 0o755))
	gw := startGateway(appDir, map[string]any{
		"db_path":         dbPath,
		"sampling_rate":   0,
		"api_listen_addr": "127.0.0.1:" + altAPIPort,
		"retention_days":  7,
	})
	defer gw.stop()

	waitHealth(altAPIURL)
	if listening(apiPort) {
		fail("API still on the default port %s with api_listen_addr set", apiPort)
	}
	if _, err := os.Stat(dbPath); err != nil {
		fail("db_path not used: %v", err)
	}
	var cfg map[string]any
	getJSON(altAPIURL+"/replaylab/config", &cfg)
	if cfg["sampling_rate"] != 0.0 || cfg["retention_days"] != 7.0 {
		fail("config not applied: %v", cfg)
	}
	for i := 0; i < 5; i++ {
		chat("gpt-4o", prompts[i%len(prompts)], nil)
	}
	time.Sleep(time.Second)
	if got := storedCaptures(altAPIURL); got != 0 {
		fail("sampling_rate 0 still captured %d requests", got)
	}
	log.Printf("  db_path, sampling_rate, retention_days and api_listen_addr applied")
}

func invalidConfig(root string, config map[string]any, wantMsg string) {
	appDir := filepath.Join(root, "invalid-"+fmt.Sprint(time.Now().UnixNano()))
	gw := startGateway(appDir, config)
	defer gw.stop()

	// bifrost-http keeps serving when a plugin fails to load; it logs the
	// plugin's Init error and marks the plugin as errored.
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(gw.logs(), wantMsg) {
		if time.Now().After(deadline) {
			fail("gateway log never mentioned %q", wantMsg)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.Contains(gw.logs(), "replaylab (built for bifrost-http "+*version+")") {
		fail("error does not name the plugin and its target gateway version")
	}
	if listening(apiPort) {
		fail("Replay Lab API is up despite invalid config")
	}
	log.Printf("  plugin rejected: %s", wantMsg)
}

// --- gateway -----------------------------------------------------------------

func startGateway(appDir string, pluginConfig map[string]any) *proc {
	must(os.MkdirAll(appDir, 0o755))
	cfg := map[string]any{
		"$schema": "https://www.getbifrost.ai/schema",
		"client": map[string]any{
			"enable_logging": false,
		},
		"config_store": map[string]any{"enabled": false},
		"logs_store":   map[string]any{"enabled": false},
		"providers": map[string]any{
			"openai": map[string]any{
				"keys": []any{map[string]any{
					"name": "mock", "value": "sk-mock", "models": []string{"*"}, "weight": 1,
				}},
				"network_config": map[string]any{"base_url": "http://127.0.0.1:" + mockPort},
			},
		},
		"plugins": []any{map[string]any{
			"name":    "replaylab",
			"path":    *pluginPath,
			"enabled": true,
			"config":  pluginConfig,
		}},
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(appDir, "config.json"), b, 0o644))

	gw := start("gateway", []string{"APP_DIR=" + appDir, "APP_PORT=" + gatewayPort},
		*gatewayBin, "-app-dir", appDir, "-port", gatewayPort, "-host", "127.0.0.1", "-log-style", "pretty")
	waitHTTP("gateway", gatewayURL+"/health")
	return gw
}

func chat(model, prompt string, headers map[string]string) {
	body, _ := json.Marshal(map[string]any{
		"model":    "openai/" + model,
		"messages": []any{map[string]any{"role": "user", "content": prompt}},
	})
	req, _ := http.NewRequest(http.MethodPost, gatewayURL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		fail("chat through gateway: %v", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fail("chat through gateway: %d %s", resp.StatusCode, out)
	}
}

// --- Replay Lab API ----------------------------------------------------------

func waitHealth(base string) map[string]any {
	waitHTTP("Replay Lab API", base+"/replaylab/health")
	var h map[string]any
	getJSON(base+"/replaylab/health", &h)
	if h["status"] != "healthy" {
		fail("health status = %v", h["status"])
	}
	return h
}

func storedCaptures(base string) int {
	var h map[string]any
	getJSON(base+"/replaylab/health", &h)
	n, _ := h["stored_captures"].(float64)
	return int(n)
}

func waitCaptures(base string, n int) {
	deadline := time.Now().Add(15 * time.Second)
	for storedCaptures(base) < n {
		if time.Now().After(deadline) {
			fail("only %d of %d captures stored", storedCaptures(base), n)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func waitExperiment(id string) map[string]any {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var e map[string]any
		getJSON(apiURL+"/replaylab/experiments/"+id, &e)
		switch e["status"] {
		case "completed", "cancelled", "failed", "budget_exhausted":
			return e
		}
		if time.Now().After(deadline) {
			fail("experiment %s still %v", id, e["status"])
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// --- plumbing ----------------------------------------------------------------

var client = &http.Client{Timeout: 30 * time.Second}

func getJSON(url string, out any) {
	resp, err := client.Get(url)
	if err != nil {
		fail("GET %s: %v", url, err)
	}
	decode("GET "+url, resp, out)
}

func postJSON(url string, in, out any) {
	b, _ := json.Marshal(in)
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		fail("POST %s: %v", url, err)
	}
	decode("POST "+url, resp, out)
}

func decode(what string, resp *http.Response, out any) {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fail("%s: %d %s", what, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		fail("%s: decoding %s: %v", what, body, err)
	}
}

func waitHTTP(what, url string) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			fail("%s never became ready at %s (last error: %v)", what, url, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func listening(port string) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// proc is a child process whose output is kept for assertions and echoed.
type proc struct {
	name string
	cmd  *exec.Cmd
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func (p *proc) Write(b []byte) (int, error) {
	p.mu.Lock()
	p.buf.Write(b)
	p.mu.Unlock()
	for _, line := range strings.SplitAfter(string(b), "\n") {
		if line != "" {
			fmt.Fprintf(os.Stderr, "    [%s] %s", p.name, line)
		}
	}
	return len(b), nil
}

func (p *proc) logs() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String()
}

var running []*proc

func start(name string, env []string, bin string, args ...string) *proc {
	p := &proc{name: name, done: make(chan struct{})}
	p.cmd = exec.Command(bin, args...)
	p.cmd.Env = append(os.Environ(), env...)
	p.cmd.Stdout, p.cmd.Stderr = p, p
	if err := p.cmd.Start(); err != nil {
		fail("starting %s: %v", name, err)
	}
	go func() { _ = p.cmd.Wait(); close(p.done) }()
	running = append(running, p)
	return p
}

// stop sends SIGTERM, so the gateway shuts down gracefully and runs the
// plugin's Cleanup, and waits for the process to exit.
func (p *proc) stop() {
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(30 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
		fail("%s did not exit within 30s of SIGTERM", p.name)
	}
}

func step(name string, fn func()) {
	log.Printf("--- %s", name)
	fn()
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	for _, p := range running {
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	}
	os.Exit(1)
}

func must(err error) {
	if err != nil {
		fail("%v", err)
	}
}
