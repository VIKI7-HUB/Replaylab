# Stale-Answer Detector API
#
# Build:  docker build -t stale-detector ./stale
# Run:    docker run --rm -p 8000:8000 --env-file stale/.env stale-detector
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1

WORKDIR /app

# Install dependencies first so the layer caches across code changes.
COPY requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

# Application code, demo, benchmark and tests.
COPY app ./app
COPY demo ./demo
COPY benchmark ./benchmark
COPY tests ./tests

# The SQLite database lives in a volume so report history survives restarts.
RUN mkdir -p /data
ENV DATABASE_PATH=/data/stale_detector.db

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8000/v1/health', timeout=4).status == 200 else 1)"

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
