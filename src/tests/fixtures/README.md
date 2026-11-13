# API fixtures

Real responses recorded from the backends, used by `../contract.test.ts`. Refresh them after a backend changes a response shape:

```bash
# Replay Lab sidecar with the mock provider (llm-reliability-backend/replaylab)
go run ./cmd/mockprovider & go run ./cmd/replaylab -port=8086 -gateway=http://localhost:8081
go run ./demo/generate_traffic -proxy http://localhost:8086 -n 30
# Stale API (llm-reliability-backend/stale): uvicorn app.main:app --port 8001

B=http://localhost:8086/replaylab S=http://localhost:8001/v1
curl -s $B/health > replaylab.health.json
curl -s $B/config > replaylab.config.json
curl -s $B/models > replaylab.models.json
curl -s "$B/captures?limit=3" > replaylab.captures.json
curl -s $B/captures/<id> > replaylab.capture.json
curl -s "$B/experiments?limit=3" > replaylab.experiments.json
curl -s $B/experiments/<id> > replaylab.experiment.json
curl -s -X POST $B/experiments -H 'content-type: application/json' \
  -d '{"name":"fixture","original_model":"gpt-4o","candidate_models":["gpt-4o-mini"],"sample_size":5,"dry_run":true}' > replaylab.estimate.json
curl -s $S/health > stale.health.json
curl -s "$S/history?limit=3" > stale.history.json
curl -s $S/history/<report_id> > stale.report.json
```
