/**
 * API contract tests: every zod schema in src/api must accept a response the
 * real backend actually sent. The fixtures in ./fixtures were recorded from the
 * running services (Replay Lab sidecar with the mock provider, and the
 * Stale-Answer Detector), so a schema that drifts from the backend fails here,
 * not in front of a user. To refresh them, see fixtures/README.md.
 */
import { describe, it, expect } from 'vitest';
import fs from 'fs';
import path from 'path';
import { z } from 'zod';
import {
  HealthSchema,
  ConfigSchema,
  ModelPriceSchema,
  CaptureSchema,
  ExperimentSchema,
  EstimateResponseSchema,
} from '../api/replaylab';
import { StaleHealthSchema, CheckReportSchema, HistoryItemSchema } from '../api/stale';

function fixture(name: string): unknown {
  return JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures', name), 'utf-8'));
}

const cases: [string, z.ZodTypeAny, string][] = [
  ['Replay Lab GET /replaylab/health', HealthSchema, 'replaylab.health.json'],
  ['Replay Lab GET /replaylab/config', ConfigSchema, 'replaylab.config.json'],
  ['Replay Lab GET /replaylab/models', z.array(ModelPriceSchema), 'replaylab.models.json'],
  ['Replay Lab GET /replaylab/captures', z.array(CaptureSchema), 'replaylab.captures.json'],
  ['Replay Lab GET /replaylab/captures/{id}', CaptureSchema, 'replaylab.capture.json'],
  ['Replay Lab GET /replaylab/experiments', z.array(ExperimentSchema), 'replaylab.experiments.json'],
  ['Replay Lab GET /replaylab/experiments/{id}', ExperimentSchema, 'replaylab.experiment.json'],
  ['Replay Lab POST /replaylab/experiments (dry run)', EstimateResponseSchema, 'replaylab.estimate.json'],
  ['Stale GET /v1/health', StaleHealthSchema, 'stale.health.json'],
  ['Stale GET /v1/history', z.array(HistoryItemSchema), 'stale.history.json'],
  ['Stale GET /v1/history/{id}', CheckReportSchema, 'stale.report.json'],
];

describe('API contract: the console reads real backend responses', () => {
  for (const [name, schema, file] of cases) {
    it(name, () => {
      const parsed = schema.safeParse(fixture(file));
      expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues, null, 2)).toBe(true);
    });
  }
});
