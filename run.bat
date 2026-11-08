@echo off
REM Windows helper. Usage: run.bat [bifrost^|demo^|test^|bench^|mock^|traffic^|run]
setlocal
if "%1"=="bifrost" (
    go run ./demo/bifrost_embedded
) else if "%1"=="demo" (
    go run ./demo/run_experiment
) else if "%1"=="test" (
    go vet ./... && go test ./tests/...
) else if "%1"=="bench" (
    go test -run "^$" -bench=. -benchmem ./tests
) else if "%1"=="mock" (
    go run ./cmd/mockprovider
) else if "%1"=="traffic" (
    go run ./demo/generate_traffic
) else (
    go run ./cmd/replaylab -port=8085 -gateway=http://localhost:8081
)
