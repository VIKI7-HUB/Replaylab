.PHONY: build test vet bench demo traffic run mock clean

build:
	go build -o bin/replaylab ./cmd/replaylab
	go build -o bin/mockprovider ./cmd/mockprovider

vet:
	go vet ./...

test: vet
	go test ./tests/...

bench:
	go test -run '^$$' -bench=. -benchmem ./tests

# Self-contained end-to-end demo (no servers or keys needed).
demo:
	go run ./demo/run_experiment

mock:
	go run ./cmd/mockprovider

run:
	go run ./cmd/replaylab -port=8085 -gateway=http://localhost:8081

# Send traffic through a running Replay Lab proxy.
traffic:
	go run ./demo/generate_traffic

clean:
	rm -rf bin/ replaylab.db replaylab.db-wal replaylab.db-shm
