.PHONY: setup run test demo benchmark lint

setup:
	pip install -r requirements.txt

run:
	uvicorn app.main:app --reload --port 8000

test:
	pytest tests/ -v

demo:
	python demo/run_demo.py

benchmark:
	python benchmark/run_benchmark.py

lint:
	python -m pyflakes app tests benchmark demo
