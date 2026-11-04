.PHONY: setup build test bench run demo traffic clean

setup:
	go mod download
	go mod tidy

build:
	go build -o bin/replaylab.exe ./cmd/replaylab
	go build -o bin/mockprovider.exe ./cmd/mockprovider

test:
	go test -v ./tests/...

bench:
	go test -bench=BenchmarkCaptureQueueEnqueue -benchmem ./tests

traffic:
	go run demo/generate_traffic.go

run: build
	./bin/replaylab.exe -port=8085

demo: traffic
	go run demo/run_experiment.go

clean:
	rm -rf bin/ replaylab.db
