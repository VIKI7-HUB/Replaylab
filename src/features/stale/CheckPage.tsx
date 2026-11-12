import React, { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ChevronRightIcon,
  CheckCircledIcon,
  CrossCircledIcon,
} from '@radix-ui/react-icons';
import { PageHeader } from '../../app/PageHeader';
import { Button } from '../../components/Button';
import { Input, Textarea } from '../../components/Input';
import { Field } from '../../components/Field';
import { Select } from '../../components/Select';
import { Checkbox } from '../../components/Checkbox';
import { Panel } from '../../components/Panel';
import { StatusLabel } from '../../components/StatusLabel';
import { Tooltip } from '../../components/Tooltip';
import { getStaleHealth, askQuestion, StaleHealth, Claim, CheckReport } from '../../api/stale';
import { streamCheck, StaleStreamEvent } from '../../api/sse';
import { useToast } from '../../components/Toast';

interface PipelineStep {
  name: string;
  status: 'pending' | 'active' | 'done' | 'failed';
  detail?: string;
}

const EXAMPLE_ANSWER =
  'Python 3.9 is the latest stable release of Python. OpenAI GPT-4 costs $0.03 per 1K input tokens. In 1969 humans landed on the Moon.';

export const CheckPage: React.FC = () => {
  const navigate = useNavigate();
  const { toast } = useToast();

  const [health, setHealth] = useState<StaleHealth | null>(null);

  // Form State
  const [answer, setAnswer] = useState('');
  const [question, setQuestion] = useState('');
  const [answerDate, setAnswerDate] = useState('');

  // Advanced options
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [maxClaims, setMaxClaims] = useState('10');
  const [depth, setDepth] = useState<'basic' | 'advanced'>('basic');
  const [allowInput, setAllowInput] = useState('');
  const [allowedDomains, setAllowedDomains] = useState<string[]>([]);
  const [blockInput, setBlockInput] = useState('');
  const [blockedDomains, setBlockedDomains] = useState<string[]>([]);
  const [skipCache, setSkipCache] = useState(false);

  // Streaming & Pipeline State
  const [isStreaming, setIsStreaming] = useState(false);
  const [errorDetail, setErrorDetail] = useState<string | null>(null);
  const [claimsList, setClaimsList] = useState<
    Array<{ id: string; text: string; status: 'queued' | 'running' | 'done'; verdict?: string | null }>
  >([]);

  const abortControllerRef = useRef<AbortController | null>(null);

  const [steps, setSteps] = useState<PipelineStep[]>([
    { name: 'Extract claims', status: 'pending' },
    { name: 'Triage by risk', status: 'pending' },
    { name: 'Verify claims', status: 'pending' },
    { name: 'Score', status: 'pending' },
    { name: 'Write correction', status: 'pending' },
  ]);

  useEffect(() => {
    getStaleHealth()
      .then((h) => setHealth(h))
      .catch(() => {});
  }, []);

  const handleAddAllowed = (val: string) => {
    const trimmed = val.trim();
    if (trimmed && !allowedDomains.includes(trimmed)) {
      setAllowedDomains([...allowedDomains, trimmed]);
      setAllowInput('');
    }
  };

  const handleAddBlocked = (val: string) => {
    const trimmed = val.trim();
    if (trimmed && !blockedDomains.includes(trimmed)) {
      setBlockedDomains([...blockedDomains, trimmed]);
      setBlockInput('');
    }
  };

  const handleCheck = async () => {
    if (answer.trim().length < 5) {
      toast('Must be at least 5 characters.', 'warning');
      return;
    }

    setIsStreaming(true);
    setErrorDetail(null);
    setClaimsList([]);

    // Reset steps
    setSteps([
      { name: 'Extract claims', status: 'active' },
      { name: 'Triage by risk', status: 'pending' },
      { name: 'Verify claims', status: 'pending' },
      { name: 'Score', status: 'pending' },
      { name: 'Write correction', status: 'pending' },
    ]);

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    let verifiedCount = 0;
    let toVerifyTotal = 0;

    try {
      await streamCheck(
        {
          answer,
          question: question.trim() || undefined,
          answer_date: answerDate || undefined,
          options: {
            max_claims: parseInt(maxClaims, 10) || 10,
            depth,
            domains_allow: allowedDomains.length > 0 ? allowedDomains : undefined,
            domains_block: blockedDomains.length > 0 ? blockedDomains : undefined,
            force_fresh: skipCache,
          },
        },
        (event: StaleStreamEvent) => {
          if (event.type === 'pipeline_started') {
            setSteps((prev) => [
              { ...prev[0], status: 'active' },
              prev[1],
              prev[2],
              prev[3],
              prev[4],
            ]);
          } else if (event.type === 'claims_extracted') {
            const count = event.data.count;
            const extractedClaims: Claim[] = event.data.claims || [];
            setSteps((prev) => [
              { ...prev[0], status: 'done', detail: `${count} claims found` },
              { ...prev[1], status: 'active' },
              prev[2],
              prev[3],
              prev[4],
            ]);
            setClaimsList(
              extractedClaims.map((c) => ({
                id: c.id,
                text: c.text,
                status: 'queued',
              }))
            );
          } else if (event.type === 'triage_completed') {
            toVerifyTotal = event.data.active;
            const skipped = event.data.skipped;
            setSteps((prev) => [
              prev[0],
              {
                ...prev[1],
                status: 'done',
                detail: `${toVerifyTotal} to verify · ${skipped} skipped`,
              },
              { ...prev[2], status: 'active' },
              prev[3],
              prev[4],
            ]);
          } else if (event.type === 'claim_verifying') {
            const id = event.data.claim_id;
            setClaimsList((prev) =>
              prev.map((c) => (c.id === id ? { ...c, status: 'running' } : c))
            );
          } else if (event.type === 'claim_verified') {
            verifiedCount++;
            const { claim_id, verdict } = event.data;
            setSteps((prev) => [
              prev[0],
              prev[1],
              {
                ...prev[2],
                status: 'active',
                detail: `${verifiedCount} of ${toVerifyTotal} verified`,
              },
              prev[3],
              prev[4],
            ]);
            setClaimsList((prev) =>
              prev.map((c) =>
                c.id === claim_id ? { ...c, status: 'done', verdict } : c
              )
            );
          } else if (event.type === 'complete') {
            const report: CheckReport = event.data;
            setSteps((prev) => [
              prev[0],
              prev[1],
              { ...prev[2], status: 'done', detail: `${toVerifyTotal} of ${toVerifyTotal} verified` },
              { ...prev[3], status: 'done' },
              { ...prev[4], status: 'done' },
            ]);
            setIsStreaming(false);
            // Navigate to report with state (§14.1)
            navigate(`/stale/reports/${encodeURIComponent(report.report_id)}`, {
              state: { report },
            });
          } else if (event.type === 'error') {
            setErrorDetail(event.data.detail);
            setSteps((prev) =>
              prev.map((s) => (s.status === 'active' ? { ...s, status: 'failed' } : s))
            );
            setIsStreaming(false);
          }
        },
        abortController.signal
      );
    } catch (err: any) {
      if (err.name === 'AbortError') return;
      setErrorDetail(err.message || 'Stream connection failed');
      setSteps((prev) =>
        prev.map((s) => (s.status === 'active' ? { ...s, status: 'failed' } : s))
      );
      setIsStreaming(false);
    }
  };

  const handleGenerateAndCheck = async () => {
    if (!question.trim()) {
      toast("Must provide a question for 'Generate and check'.", 'warning');
      return;
    }
    setIsStreaming(true);
    try {
      const report = await askQuestion(question);
      navigate(`/stale/reports/${encodeURIComponent(report.report_id)}`, {
        state: { report },
      });
    } catch (err: any) {
      toast(err.message || "Couldn't generate and check answer", 'danger');
      setIsStreaming(false);
    }
  };

  const renderStepMarker = (status: PipelineStep['status']) => {
    if (status === 'done') {
      return <CheckCircledIcon className="w-[15px] h-[15px] text-success shrink-0" />;
    }
    if (status === 'failed') {
      return <CrossCircledIcon className="w-[15px] h-[15px] text-danger shrink-0" />;
    }
    if (status === 'active') {
      return (
        <span className="w-3 h-3 flex items-center justify-center shrink-0">
          <span className="w-1.5 h-1.5 rounded-full bg-accent animate-running-dot" />
        </span>
      );
    }
    return (
      <span className="w-3 h-3 rounded-full border-[1.5px] border-border-strong inline-block shrink-0" />
    );
  };

  return (
    <div>
      <PageHeader
        title="Check an answer"
        description="Find claims that may be out of date and verify them against live sources."
      />

      <div className="flex flex-col lg:flex-row gap-8 items-start">
        {/* Left Column: minimum 520px wide form */}
        <div className="w-full lg:min-w-[520px] lg:flex-1 space-y-6">
          <Field
            label="Answer"
            helperText="Provide the AI response text to check for stale information."
            error={
              answer.length > 20000
                ? "Can't exceed 20,000 characters."
                : undefined
            }
          >
            <div className="relative">
              <Textarea
                className="mono-ui text-[13px] min-h-[240px]"
                value={answer}
                onChange={(e) => setAnswer(e.target.value)}
                maxLength={20000}
                placeholder="Paste the answer to audit here…"
                disabled={isStreaming}
              />
              <div className="text-[12px] text-text-3 text-right mt-1 tabular-nums">
                {answer.length.toLocaleString('en-US')} / 20,000
              </div>
            </div>
          </Field>

          <Field label="Question" helperText="Optional. The question that prompted the answer.">
            <Input
              value={question}
              onChange={(e) => setQuestion(e.target.value)}
              placeholder="e.g. What is the latest release of Python?"
              disabled={isStreaming}
            />
          </Field>

          <Field
            label="Answer date"
            helperText="When the answer was written. Older answers get a higher staleness risk."
          >
            <Input
              type="date"
              value={answerDate}
              onChange={(e) => setAnswerDate(e.target.value)}
              disabled={isStreaming}
            />
          </Field>

          {/* Collapsible Advanced section */}
          <div>
            <button
              type="button"
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="inline-flex items-center gap-1.5 text-[13px] font-medium text-text-2 hover:text-text cursor-pointer py-1"
            >
              <ChevronRightIcon
                className={`w-4 h-4 transition-transform ${showAdvanced ? 'rotate-90' : ''}`}
              />
              Advanced options
            </button>

            {showAdvanced && (
              <div className="space-y-4 pt-3 pl-5 border-l border-border mt-1">
                <Field label="Max claims" helperText="Limit number of extracted claims (1 - 30).">
                  <Input
                    type="number"
                    min="1"
                    max="30"
                    value={maxClaims}
                    onChange={(e) => setMaxClaims(e.target.value)}
                  />
                </Field>

                <Field label="Depth">
                  <Select
                    value={depth}
                    onValueChange={(val) => setDepth(val as 'basic' | 'advanced')}
                    options={[
                      { value: 'basic', label: 'Basic' },
                      { value: 'advanced', label: 'Advanced' },
                    ]}
                  />
                </Field>

                <Field label="Allowed domains" helperText="Press Enter to add domain chip.">
                  <div className="flex flex-wrap gap-1.5 p-1.5 border border-border-strong rounded bg-bg">
                    {allowedDomains.map((d) => (
                      <span
                        key={d}
                        className="mono-ui text-[12px] bg-bg-muted text-text px-1.5 py-0.5 rounded-sm inline-flex items-center gap-1"
                      >
                        {d}
                        <button
                          type="button"
                          onClick={() =>
                            setAllowedDomains(allowedDomains.filter((item) => item !== d))
                          }
                          className="text-text-3 hover:text-text"
                        >
                          ×
                        </button>
                      </span>
                    ))}
                    <input
                      type="text"
                      value={allowInput}
                      onChange={(e) => setAllowInput(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          handleAddAllowed(allowInput);
                        }
                      }}
                      placeholder={allowedDomains.length === 0 ? 'e.g. openai.com' : ''}
                      className="flex-1 min-w-[120px] bg-transparent text-[13px] text-text outline-none"
                    />
                  </div>
                </Field>

                <Field label="Blocked domains" helperText="Press Enter to add domain chip.">
                  <div className="flex flex-wrap gap-1.5 p-1.5 border border-border-strong rounded bg-bg">
                    {blockedDomains.map((d) => (
                      <span
                        key={d}
                        className="mono-ui text-[12px] bg-bg-muted text-text px-1.5 py-0.5 rounded-sm inline-flex items-center gap-1"
                      >
                        {d}
                        <button
                          type="button"
                          onClick={() =>
                            setBlockedDomains(blockedDomains.filter((item) => item !== d))
                          }
                          className="text-text-3 hover:text-text"
                        >
                          ×
                        </button>
                      </span>
                    ))}
                    <input
                      type="text"
                      value={blockInput}
                      onChange={(e) => setBlockInput(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          handleAddBlocked(blockInput);
                        }
                      }}
                      placeholder={blockedDomains.length === 0 ? 'e.g. reddit.com' : ''}
                      className="flex-1 min-w-[120px] bg-transparent text-[13px] text-text outline-none"
                    />
                  </div>
                </Field>

                <Checkbox
                  id="skip-cache"
                  checked={skipCache}
                  onCheckedChange={(checked) => setSkipCache(Boolean(checked))}
                  label="Skip cache"
                />
              </div>
            )}
          </div>

          {/* Form Actions */}
          <div className="flex flex-wrap items-center gap-3 pt-2">
            <Button
              size="md"
              variant="primary"
              loading={isStreaming}
              onClick={handleCheck}
              disabled={isStreaming || answer.trim().length < 5}
            >
              Check answer
            </Button>

            <Tooltip
              content={
                health && !health.llm_configured
                  ? 'Needs an LLM. Set LLM_API_KEY on the service.'
                  : undefined
              }
            >
              <div>
                <Button
                  size="md"
                  variant="secondary"
                  disabled={!health?.llm_configured || isStreaming}
                  onClick={handleGenerateAndCheck}
                >
                  Generate and check
                </Button>
              </div>
            </Tooltip>

            <Button
              size="sm"
              variant="ghost"
              disabled={isStreaming}
              onClick={() => {
                setAnswer(EXAMPLE_ANSWER);
                setQuestion('What are current specs and dates for Python and GPT-4?');
              }}
            >
              Load example
            </Button>
          </div>
        </div>

        {/* Right Column: 360px wide Pipeline tracker panel */}
        <div className="w-full lg:w-[360px] space-y-4">
          <Panel title="Progress">
            <div className="space-y-4">
              {steps.map((step, idx) => (
                <div key={idx} className="flex items-start gap-3">
                  <div className="mt-0.5">{renderStepMarker(step.status)}</div>
                  <div className="flex-1">
                    <div className="text-[13px] text-text font-medium leading-tight">
                      {step.name}
                    </div>
                    {step.detail && (
                      <div className="text-[12px] text-text-3 mt-0.5">{step.detail}</div>
                    )}
                  </div>
                </div>
              ))}

              {errorDetail && (
                <div className="text-[12px] text-danger bg-danger-bg p-2.5 rounded border border-danger mt-2">
                  {errorDetail}
                </div>
              )}
            </div>
          </Panel>

          {/* Streaming claims list */}
          {claimsList.length > 0 && (
            <Panel title="Claims" noPadding>
              <div className="divide-y divide-border max-h-[320px] overflow-y-auto">
                {claimsList.map((c) => (
                  <div key={c.id} className="p-3 text-[13px] space-y-1.5">
                    <div className="text-text line-clamp-2">{c.text}</div>
                    <div>
                      {c.status === 'queued' ? (
                        <StatusLabel value="queued" customText="Queued" />
                      ) : c.status === 'running' ? (
                        <StatusLabel value="running" customText="Verifying" />
                      ) : (
                        <StatusLabel value={c.verdict || 'null'} />
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </Panel>
          )}
        </div>
      </div>
    </div>
  );
};

