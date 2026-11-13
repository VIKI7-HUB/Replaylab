.PHONY: build test vet bench demo demo-bifrost traffic run mock clean loadable loadable-smoke loadable-vet gateway-demo

build:
	go build -o bin/replaylab ./cmd/replaylab
	go build -o bin/mockprovider ./cmd/mockprovider

vet:
	go vet ./...

test: vet
	go test ./tests/...

bench:
	go test -run '^$$' -bench=. -benchmem ./tests ./plugins/replaylab

# Replay Lab inside the real Bifrost core (no keys needed).
demo-bifrost:
	go run ./demo/bifrost_embedded

# Self-contained sidecar demo (no servers or keys needed).
demo:
	go run ./demo/run_experiment

mock:
	go run ./cmd/mockprovider

run:
	go run ./cmd/replaylab -port=8085 -gateway=http://localhost:8081

# Send traffic through a running Replay Lab proxy.
traffic:
	go run ./demo/generate_traffic

# Loadable plugin for bifrost-http (needs Docker; shared objects are Linux/macOS
# only). The target gateway version is the transports requirement in
# loadable/go.mod.
GATEWAY_VERSION := $(shell awk '$$1 == "github.com/maximhq/bifrost/transports" { print $$2; exit }' loadable/go.mod)

# Build the shared object into dist/replaylab-bifrost-http-<version>.so.
loadable:
	docker build -f loadable/Dockerfile --target plugin -o dist .
	@echo "built dist/replaylab-bifrost-http-$(GATEWAY_VERSION).so"

# Build the target gateway and the plugin, then run the gateway smoke test.
loadable-smoke:
	docker build -f loadable/Dockerfile --target smoke .

# Real bifrost-http with Replay Lab loaded, the mock provider upstream, and
# seeded traffic plus an experiment. No API keys needed.
gateway-demo:
	docker compose -f loadable/compose.yml up --build

# Type-check the loadable target on any OS (no cgo, no plugin build mode), on
# the gateway's Go version. Like the container build, it works on a copy whose
# shared-core go.mod is replaced (loadable/prepare-core.sh).
GATEWAY_GO := $(shell awk '$$1 == "go" { print $$2; exit }' loadable/go.mod)
VET_DIR := $(or $(TMPDIR),/tmp)/replaylab-loadable-vet
loadable-vet:
	rm -rf "$(VET_DIR)" && mkdir -p "$(VET_DIR)"
	cp -r go.mod go.sum plugins loadable "$(VET_DIR)/"
	sh loadable/prepare-core.sh "$(VET_DIR)" $(GATEWAY_GO)
	cd "$(VET_DIR)/loadable" && GOTOOLCHAIN=go$(GATEWAY_GO) GOOS=linux CGO_ENABLED=0 		go vet -tags replaylab_core_v1_7 . ./smoketest

clean:
	rm -rf bin/ dist/ replaylab.db replaylab.db-wal replaylab.db-shm
