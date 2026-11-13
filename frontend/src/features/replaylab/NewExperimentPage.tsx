import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { PageHeader } from '../../app/PageHeader';
import { Button } from '../../components/Button';
import { Input, Textarea } from '../../components/Input';
import { Field } from '../../components/Field';
import { Select } from '../../components/Select';
import { Checkbox } from '../../components/Checkbox';
import { Switch } from '../../components/Switch';
import { Panel } from '../../components/Panel';
import {
  getModels,
  getCaptures,
  getConfig,
  createExperiment,
  ModelPrice,
  EstimateResponse,
} from '../../api/replaylab';
import { formatMoney } from '../../lib/format';
import { useToast } from '../../components/Toast';
import { errorMessage } from '../../api/error';

export const NewExperimentPage: React.FC = () => {
  const navigate = useNavigate();
  const { toast } = useToast();

  const [models, setModels] = useState<ModelPrice[]>([]);
  const [defaultBudget, setDefaultBudget] = useState<number>(10);

  // Form State
  const [name, setName] = useState('');
  const [baselineModel, setBaselineModel] = useState('');
  const [tagFilter, setTagFilter] = useState('');
  const [trafficFilter, setTrafficFilter] = useState('all');

  const [candidateModels, setCandidateModels] = useState<string[]>([]);
  const [sampleSize, setSampleSize] = useState('50');
  const [budgetCap, setBudgetCap] = useState('10');
  const [temperature, setTemperature] = useState('');
  const [monthlyVolume, setMonthlyVolume] = useState('');

  // Judge State
  const [enableJudge, setEnableJudge] = useState(false);
  const [judgeModel, setJudgeModel] = useState('');
  const [rubric, setRubric] = useState('');
  const [maxJudgeSamples, setMaxJudgeSamples] = useState('');

  // Estimate State
  const [estimate, setEstimate] = useState<EstimateResponse | null>(null);
  const [estimating, setEstimating] = useState(false);
  const [starting, setStarting] = useState(false);

  useEffect(() => {
    // Default the baseline to the model of the newest capture, so the form
    // starts on traffic that exists. With no match it stays on "Any model".
    Promise.all([getModels(), getCaptures({ limit: 1 }).catch(() => [])])
      .then(([m, latest]) => {
        setModels(m);
        const capture = latest[0];
        if (!capture) return;
        const names = [capture.model, `${capture.provider ?? ''}/${capture.model}`];
        const match = m.find((model) => names.includes(model.model));
        if (match) setBaselineModel((current) => current || match.model);
      })
      .catch(() => {});

    getConfig()
      .then((cfg) => {
        if (cfg.default_budget_cap) {
          setDefaultBudget(cfg.default_budget_cap);
          setBudgetCap(String(cfg.default_budget_cap));
        }
      })
      .catch(() => {});
  }, []);

  // Form validation
  const candidateError =
    candidateModels.length === 0
      ? "Can't start without selecting candidate models."
      : candidateModels.length > 10
      ? 'Must be between 1 and 10 models.'
      : undefined;

  // Clear estimate on any form change (§12.3: Any change to the form clears the estimate)
  const onFormChange = (callback: () => void) => {
    setEstimate(null);
    callback();
  };

  const handleToggleCandidate = (modelName: string) => {
    onFormChange(() => {
      setCandidateModels((prev) =>
        prev.includes(modelName) ? prev.filter((m) => m !== modelName) : [...prev, modelName]
      );
    });
  };

  const availableCandidates = models.filter((m) => m.model !== baselineModel);
  const availableJudgeModels = models.filter(
    (m) => !candidateModels.includes(m.model) && m.model !== baselineModel
  );

  const getPayload = (isDryRun: boolean) => ({
    name: name.trim() || `${baselineModel || 'any model'} vs ${candidateModels.join(', ')}`.slice(0, 120),
    original_model: baselineModel,
    candidate_models: candidateModels,
    filter_tag: tagFilter.trim() || undefined,
    streamed:
      trafficFilter === 'streamed' ? true : trafficFilter === 'non_streamed' ? false : undefined,
    sample_size: parseInt(sampleSize, 10) || 50,
    budget_cap_dollars: parseFloat(budgetCap) || defaultBudget,
    fixed_temperature: temperature !== '' ? parseFloat(temperature) : undefined,
    monthly_request_volume: monthlyVolume !== '' ? parseInt(monthlyVolume, 10) : undefined,
    dry_run: isDryRun,
    judge: enableJudge && judgeModel
      ? {
          model: judgeModel,
          rubric: rubric.slice(0, 500),
          max_judge_samples: maxJudgeSamples !== '' ? parseInt(maxJudgeSamples, 10) : undefined,
        }
      : undefined,
  });

  const handleEstimate = async () => {
    if (candidateModels.length === 0 || candidateModels.length > 10) {
      toast('Please choose 1 to 10 candidate models.', 'warning');
      return;
    }

    setEstimating(true);
    try {
      const res = await createExperiment(getPayload(true));
      setEstimate(res as EstimateResponse);
    } catch (err) {
      toast(errorMessage(err, "Couldn't estimate cost"), 'danger');
    } finally {
      setEstimating(false);
    }
  };

  const handleRun = async () => {
    if (candidateModels.length === 0 || candidateModels.length > 10) {
      toast('Please choose 1 to 10 candidate models.', 'warning');
      return;
    }

    setStarting(true);
    try {
      const res = await createExperiment(getPayload(false));
      const expId = (res as { id: string }).id;
      toast('Experiment started', 'success');
      navigate(`/replaylab/experiments/${encodeURIComponent(expId)}`);
    } catch (err) {
      toast(errorMessage(err, "Couldn't start experiment"), 'danger');
      setStarting(false);
    }
  };

  return (
    <div>
      <PageHeader
        title="New experiment"
        description="Configure baseline and candidate models to replay captured traffic under a budget cap."
      />

      <div className="flex flex-col lg:flex-row gap-8 items-start">
        {/* Left Column: 560px wide form */}
        <div className="w-full lg:w-[560px] space-y-6">
          {/* Section 1: Basics */}
          <div>
            <div className="section-title text-text mb-4">Basics</div>
            <div className="space-y-4">
              <Field label="Name" helperText="A memorable name for this experiment.">
                <Input
                  value={name}
                  onChange={(e) => onFormChange(() => setName(e.target.value))}
                  placeholder="e.g. gpt-4o vs claude-3-5-sonnet"
                />
              </Field>

              <Field label="Baseline model">
                <Select
                  value={baselineModel}
                  onValueChange={(val) =>
                    onFormChange(() => {
                      setBaselineModel(val);
                      setCandidateModels((prev) => prev.filter((m) => m !== val));
                    })
                  }
                  options={[
                    { value: '', label: 'Any model' },
                    ...models.map((m) => ({ value: m.model, label: m.model })),
                  ]}
                  placeholder="Select baseline model"
                />
              </Field>

              <Field label="Tag filter" helperText="Optional. Only replay captures matching this tag.">
                <Input
                  value={tagFilter}
                  onChange={(e) => onFormChange(() => setTagFilter(e.target.value))}
                  placeholder="e.g. production, chat, rag"
                />
              </Field>

              <Field label="Traffic">
                <Select
                  value={trafficFilter}
                  onValueChange={(val) => onFormChange(() => setTrafficFilter(val))}
                  options={[
                    { value: 'all', label: 'All traffic' },
                    { value: 'streamed', label: 'Streamed only' },
                    { value: 'non_streamed', label: 'Non-streamed only' },
                  ]}
                />
              </Field>
            </div>
          </div>

          <div className="border-t border-border" />

          {/* Section 2: Candidates */}
          <div>
            <div className="section-title text-text mb-2">Candidates</div>
            <p className="text-[12px] text-text-3 mb-3">Choose 1 to 10 models.</p>
            {candidateError && (
              <p className="text-[12px] text-danger mb-3">{candidateError}</p>
            )}
            <div className="space-y-2.5 max-h-60 overflow-y-auto p-1 border border-border rounded-md bg-bg">
              {availableCandidates.length === 0 ? (
                <div className="text-[13px] text-text-3 p-3">No candidate models available</div>
              ) : (
                availableCandidates.map((m) => (
                  <div
                    key={m.model}
                    className="flex items-center justify-between p-2 rounded hover:bg-bg-muted"
                  >
                    <Checkbox
                      id={`cand-${m.model}`}
                      checked={candidateModels.includes(m.model)}
                      onCheckedChange={() => handleToggleCandidate(m.model)}
                      label={<span className="mono-ui text-[13px]">{m.model}</span>}
                    />
                    <span className="text-[12px] text-text-3">
                      ${m.input_per_1m_tokens} / ${m.output_per_1m_tokens} per 1M
                    </span>
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="border-t border-border" />

          {/* Section 3: Sample and budget */}
          <div>
            <div className="section-title text-text mb-4">Sample and budget</div>
            <div className="space-y-4">
              <Field label="Sample size" helperText="Number of captures to replay (max 1000).">
                <Input
                  type="number"
                  min="1"
                  max="1000"
                  value={sampleSize}
                  onChange={(e) => onFormChange(() => setSampleSize(e.target.value))}
                />
              </Field>

              <Field label="Budget cap" helperText="Replays stop when this dollar amount is spent.">
                <Input
                  type="number"
                  step="0.01"
                  unit="$"
                  value={budgetCap}
                  onChange={(e) => onFormChange(() => setBudgetCap(e.target.value))}
                />
              </Field>

              <Field
                label="Temperature"
                helperText="Leave empty to use each capture's temperature"
              >
                <Input
                  type="number"
                  step="0.1"
                  min="0"
                  max="2"
                  value={temperature}
                  onChange={(e) => onFormChange(() => setTemperature(e.target.value))}
                  placeholder="0.0 - 2.0"
                />
              </Field>

              <Field
                label="Monthly volume"
                helperText="Leave empty to project from captured traffic"
              >
                <Input
                  type="number"
                  value={monthlyVolume}
                  onChange={(e) => onFormChange(() => setMonthlyVolume(e.target.value))}
                  placeholder="e.g. 100000"
                />
              </Field>
            </div>
          </div>

          <div className="border-t border-border" />

          {/* Section 4: Judge (optional) */}
          <div>
            <div className="section-title text-text mb-4">Judge (optional)</div>
            <div className="space-y-4">
              <Switch
                id="enable-judge"
                checked={enableJudge}
                onCheckedChange={(checked) => onFormChange(() => setEnableJudge(checked))}
                label="Score replays with a judge model"
              />

              {enableJudge && (
                <div className="space-y-4 pt-2 animate-fade-in">
                  <Field label="Judge model">
                    <Select
                      value={judgeModel}
                      onValueChange={(val) => onFormChange(() => setJudgeModel(val))}
                      options={availableJudgeModels.map((m) => ({
                        value: m.model,
                        label: m.model,
                      }))}
                      placeholder="Select judge model"
                    />
                  </Field>

                  <Field
                    label="Rubric"
                    helperText="Evaluation criteria for the judge (up to 500 characters)."
                  >
                    <div>
                      <Textarea
                        rows={3}
                        maxLength={500}
                        value={rubric}
                        onChange={(e) => onFormChange(() => setRubric(e.target.value))}
                        placeholder="Evaluate accuracy, conciseness, and tone compared to baseline."
                      />
                      <div className="text-[12px] text-text-3 text-right mt-1">
                        {rubric.length} / 500
                      </div>
                    </div>
                  </Field>

                  <Field label="Max judged samples" helperText="Leave empty to judge all samples.">
                    <Input
                      type="number"
                      min="1"
                      value={maxJudgeSamples}
                      onChange={(e) => onFormChange(() => setMaxJudgeSamples(e.target.value))}
                      placeholder="e.g. 20"
                    />
                  </Field>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Right Column: 320px wide sticky Estimate panel */}
        <div className="w-full lg:w-[320px] lg:sticky lg:top-8">
          <Panel title="Estimate">
            {!estimate ? (
              <div className="space-y-4">
                <p className="text-[13px] text-text-2">
                  Run a dry run to see what this experiment will cost.
                </p>
                <Button
                  size="md"
                  variant="primary"
                  className="w-full"
                  loading={estimating}
                  disabled={candidateModels.length === 0 || candidateModels.length > 10}
                  onClick={handleEstimate}
                >
                  Estimate cost
                </Button>
              </div>
            ) : (
              <div className="space-y-4">
                <div className="space-y-2.5">
                  <div className="flex items-center justify-between text-[13px]">
                    <span className="text-text-2">Replays</span>
                    <span className="text-text tabular-nums font-medium">
                      {estimate.total_replays.toLocaleString('en-US')}
                    </span>
                  </div>

                  <div>
                    <div className="text-[12px] text-text-2">Estimated cost</div>
                    <div className="stat text-text mt-0.5">
                      {formatMoney(estimate.spent_dollars)}
                    </div>
                  </div>

                  {estimate.estimated_replay_dollars !== undefined &&
                    estimate.estimated_replay_dollars !== null && (
                      <div className="flex items-center justify-between text-[13px]">
                        <span className="text-text-2">Replay cost</span>
                        <span className="text-text tabular-nums">
                          {formatMoney(estimate.estimated_replay_dollars)}
                        </span>
                      </div>
                    )}

                  {estimate.estimated_judge_dollars !== undefined &&
                    estimate.estimated_judge_dollars !== null && (
                      <div className="flex items-center justify-between text-[13px]">
                        <span className="text-text-2">Judge cost</span>
                        <span className="text-text tabular-nums">
                          {formatMoney(estimate.estimated_judge_dollars)}
                        </span>
                      </div>
                    )}

                  <div className="flex items-center justify-between text-[13px] border-t border-border pt-2">
                    <span className="text-text-2">Budget cap</span>
                    <span className="text-text tabular-nums font-medium">
                      {formatMoney(parseFloat(budgetCap) || defaultBudget)}
                    </span>
                  </div>

                  {estimate.stop_reason && (
                    <div className="text-[12px] text-warning bg-warning-bg p-2 rounded border border-warning mt-2">
                      This estimate exceeds the cap. The run would stop early.
                    </div>
                  )}
                </div>

                <div className="pt-2 space-y-2">
                  <Button
                    size="md"
                    variant="primary"
                    className="w-full"
                    loading={starting}
                    onClick={handleRun}
                  >
                    Run for ≈ {formatMoney(estimate.spent_dollars)}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="w-full"
                    onClick={handleEstimate}
                  >
                    Re-estimate
                  </Button>
                </div>
              </div>
            )}
          </Panel>
        </div>
      </div>
    </div>
  );
};

