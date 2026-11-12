@echo off
if "%1"=="setup" (
    pip install -r requirements.txt
) else if "%1"=="test" (
    pytest tests/ -v
) else if "%1"=="demo" (
    python demo/run_demo.py
) else if "%1"=="benchmark" (
    python benchmark/run_benchmark.py
) else (
    uvicorn app.main:app --reload --port 8000
)
