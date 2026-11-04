@echo off
set "PATH=%PATH%;C:\Program Files\Go\bin"

if "%1"=="setup" (
    go mod download
    go mod tidy
) else if "%1"=="test" (
    go test -v ./tests/...
) else if "%1"=="bench" (
    go test -bench=BenchmarkCaptureQueueEnqueue -benchmem ./tests
) else if "%1"=="traffic" (
    go run demo/generate_traffic.go
) else if "%1"=="demo" (
    go run demo/generate_traffic.go
    go run demo/run_experiment.go
) else (
    go run cmd/replaylab/main.go -port=8085
)
