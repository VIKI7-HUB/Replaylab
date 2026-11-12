import React, { useState, useEffect, useRef } from 'react';
import { useParams } from 'react-router-dom';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
} from 'recharts';
import {
  ChevronRightIcon,
  CopyIcon,
  DownloadIcon,
  CheckIcon,
} from '@radix-ui/react-icons';
import { PageHeader } from '../../app/PageHeader';
import { Button } from '../../components/Button';
import { Panel } from '../../components/Panel';
import { StatStrip } from '../../components/StatStrip';
import { StatusLabel } from '../../components/StatusLabel';
import { Dialog } from '../../components/Dialog';
import { Tabs } from '../../components/Tabs';
import { ChartContainer, CustomChartTooltip } from '../../components/Chart';
import { ErrorState, Skeleton } from '../../components/States';
import {
  Experiment,
  ProgressEvent,
  getExperimentById,
  cancelExperiment,
  getExperimentReport,
} from '../../api/replaylab';
import { subscribeExperiment } from '../../api/sse';
import {
  formatDuration,
  formatMoney,
  formatPercent,
  formatPercentDelta,
  formatRelativeTime,
} from '../../lib/format';
import { useToast } from '../../components/Toast';
import { errorMessage } from '../../api/error';

export const ExperimentDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const { toast } = useToast();

  const [experiment, setExperiment] = useState<Experiment | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Live progress state
  const [progress, setProgress] = useState<ProgressEvent | null>(null);
  const [logs, setLogs] = useState<Array<{ time: string; message: string }>>([]);
  const logContainerRef = useRef<HTMLDivElement>(null);

  // Dialog & copy state
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [copiedMd, setCopiedMd] = useState(false);

  // Expanded reasoning state
  const [expandedReasoning, setExpandedReasoning] = useState<Record<string, boolean>>({});

  // Judge sample expanded state
  const [expandedSamples, setExpandedSamples] = useState<Record<string, boolean>>({});

  const fetchDetails = async () => {
    if (!id) return;
    try {
      const exp = await getExperimentById(id);
      setExperiment(exp);
    } catch (err) {
      setError(errorMessage(err, 'Failed to load experiment'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDetails();
  }, [id]);

  // Subscribe to live SSE events if experiment is running
  useEffect(() => {
    if (!id || !experiment || experiment.status !== 'running') return;

    const unsubscribe = subscribeExperiment(
      id,
      (ev) => {
        setProgress(ev);
        if (ev.message) {
          const now = new Date().toLocaleTimeString();
          setLogs((prev) => [...prev.slice(-199), { time: now, message: ev.message }]);
        }
        if (ev.final) {
          fetchDetails();
        }
      },
      (err) => {
        console.warn('SSE error:', err);
      }
    );

    return () => unsubscribe();
  }, [id, experiment?.status]);

  // Auto-scroll logs
  useEffect(() => {
    if (logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
    }
  }, [logs]);

  const handleCancel = async () => {
    if (!id) return;
    setCancelling(true);
    try {
      await cancelExperiment(id);
      toast('Experiment cancelled', 'neutral');
      setCancelDialogOpen(false);
      fetchDetails();
    } catch (err) {
      toast(errorMessage(err, 'Failed to cancel run'), 'danger');
    } finally {
      setCancelling(false);
    }
  };

  const handleCopyMarkdown = async () => {
    if (!id) return;
    try {
      const md = await getExperimentReport(id);
      await navigator.clipboard.writeText(md);
      setCopiedMd(true);
      toast('Report copied to clipboard', 'success');
      setTimeout(() => setCopiedMd(false), 1500);
    } catch (err) {
      toast(errorMessage(err, 'Failed to get report'), 'danger');
    }
  };

  const handleDownloadMarkdown = async () => {
    if (!id) return;
    try {
      const md = await getExperimentReport(id);
      const blob = new Blob([md], { type: 'text/markdown' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `experiment-${id}.md`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      toast(errorMessage(err, 'Failed to download report'), 'danger');
    }
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <div className="h-8 w-64 bg-bg-muted rounded animate-skeleton" />
        <div className="h-4 w-96 bg-bg-muted rounded animate-skeleton" />
        <Skeleton height="h-64" />
      </div>
    );
  }

  if (error || !experiment) {
    return (
      <ErrorState
        title="Couldn't load experiment"
        message={error || 'Experiment not found'}
        onRetry={fetchDetails}
      />
    );
  }

  const isRunning = experiment.status === 'running';

  // Format stop reason notice
  let stopNotice = null;
  if (experiment.stop_reason) {
    const isInterrupted = experiment.stop_reason.startsWith('interrupted:');
    const isBudget = experiment.status === 'budget_exhausted';
    const borderTone = isBudget ? 'border-l-warning' : 'border-l-danger';
    const textMsg = isInterrupted
      ? 'Interrupted by a restart. Partial results are shown.'
      : experiment.stop_reason;

    stopNotice = (
      <div className={`p-3 bg-bg border border-border border-l-[3px] ${borderTone} text-[13px] text-text mb-6 rounded-md`}>
        {textMsg}
      </div>
    );
  }

  const hasJudgeData = experiment.results.some(
    (r) => r.judge_score !== null && r.judge_score !== undefined
  );

  // Prepare chart data: Cost per candidate (horizontal bars, baseline first)
  const baselineCost = experiment.results[0]?.original_cost_dollars || 0;
  const costChartData = [
    {
      name: `${experiment.original_model} (baseline)`,
      cost: baselineCost,
      color: 'var(--chart-5)',
    },
    ...experiment.results.map((r, i) => ({
      name: r.candidate_model,
      cost: r.candidate_cost_dollars,
      color: `var(--chart-${(i % 5) + 1})`,
    })),
  ];

  // Latency chart data: p50 and p95 grouped vertical bars
  const latencyChartData = experiment.results.map((r) => ({
    name: r.candidate_model,
    p50: r.p50_latency_ms,
    p95: r.p95_latency_ms,
  }));

  return (
    <div>
      {/* Header */}
      <PageHeader
        title={experiment.name}
        statusLabel={<StatusLabel value={experiment.status} stopReason={experiment.stop_reason} />}
        description={`${experiment.id} · baseline ${experiment.original_model} · created ${formatRelativeTime(
          experiment.created_at
        )}`}
        actions={
          isRunning ? (
            <Button size="md" variant="danger" onClick={() => setCancelDialogOpen(true)}>
              Cancel run
            </Button>
          ) : (
            <div className="flex items-center gap-2">
              <Button size="md" variant="secondary" onClick={handleCopyMarkdown}>
                {copiedMd ? (
                  <CheckIcon className="w-[15px] h-[15px] text-success mr-1.5" />
                ) : (
                  <CopyIcon className="w-[15px] h-[15px] mr-1.5" />
                )}
                Copy Markdown
              </Button>
              <Button size="md" variant="secondary" onClick={handleDownloadMarkdown}>
                <DownloadIcon className="w-[15px] h-[15px] mr-1.5" />
                Download .md
              </Button>
            </div>
          )
        }
      />

      {/* Cancel confirmation dialog */}
      <Dialog
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
        title="Cancel this experiment?"
        description="Replays stop now. Results so far are kept."
        cancelLabel="Keep running"
        confirmLabel="Cancel run"
        confirmVariant="danger"
        loading={cancelling}
        onConfirm={handleCancel}
      />

      {/* Stop reason notice */}
      {!isRunning && stopNotice}

      {/* WHILE RUNNING VIEW */}
      {isRunning && (
        <div className="space-y-6">
          {/* Progress bar panel */}
          <Panel>
            <div className="space-y-2">
              <div className="flex items-center justify-between text-[13px] text-text">
                <span>
                  {progress?.completed_count || experiment.completed_replays} of{' '}
                  {progress?.total_count || experiment.total_replays} replays
                </span>
                <span className="tabular-nums font-medium">
                  {progress?.progress_percent ||
                    (experiment.total_replays > 0
                      ? Math.round(
                          (experiment.completed_replays / experiment.total_replays) * 100
                        )
                      : 0)}
                  %
                </span>
              </div>
              <div className="w-full h-1.5 bg-bg-muted rounded-none overflow-hidden">
                <div
                  className="h-full bg-text transition-all duration-150"
                  style={{
                    width: `${
                      progress?.progress_percent ||
                      (experiment.total_replays > 0
                        ? (experiment.completed_replays / experiment.total_replays) * 100
                        : 0)
                    }%`,
                  }}
                />
              </div>
            </div>
          </Panel>

          {/* Running Stat strip */}
          <StatStrip
            cells={[
              {
                label: 'Spent',
                value: formatMoney(progress?.spent_dollars ?? experiment.spent_dollars),
                detail: `of ${formatMoney(experiment.budget_cap_dollars)} cap`,
              },
              {
                label: 'Current model',
                value: (
                  <span className="mono-ui text-[14px]">
                    {progress?.candidate_model || '—'}
                  </span>
                ),
              },
              {
                label: 'Similarity',
                value:
                  progress?.current_quality !== undefined && progress.current_quality !== null
                    ? formatPercent(progress.current_quality)
                    : '—',
              },
              {
                label: 'Judge score',
                value:
                  progress?.current_judge_score !== undefined &&
                  progress.current_judge_score !== null
                    ? progress.current_judge_score
                    : 'Not judged',
              },
            ]}
          />

          {/* Event log panel */}
          <Panel title="Event log">
            <div
              ref={logContainerRef}
              className="h-60 overflow-y-auto space-y-1 font-mono text-[12px] bg-bg-subtle p-3 rounded-md border border-border"
            >
              {logs.length === 0 ? (
                <div className="text-text-3">Waiting for events…</div>
              ) : (
                logs.map((log, i) => (
                  <div key={i} className="flex gap-2">
                    <span className="text-text-3 shrink-0">{log.time}</span>
                    <span className="text-text-2">{log.message}</span>
                  </div>
                ))
              )}
            </div>
          </Panel>
        </div>
      )}

      {/* FINISHED VIEW */}
      {!isRunning && (
        <div className="space-y-6">
          {/* 1. Verdict Row: CSS grid repeat(auto-fill, minmax(280px, 1fr)) */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {experiment.results.map((r) => {
              const isExpanded = expandedReasoning[r.candidate_model] || false;
              const hasSavings = r.projected_monthly_savings > 0;

              return (
                <div
                  key={r.candidate_model}
                  className="bg-bg border border-border rounded-md p-4 flex flex-col justify-between"
                >
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <span className="mono-ui text-[13px] font-medium text-text">
                        {r.candidate_model}
                      </span>
                      <StatusLabel value={r.recommendation} />
                    </div>

                    <div className="mb-3">
                      {hasSavings ? (
                        <div className="stat text-text">
                          Saves {formatMoney(r.projected_monthly_savings)} / month
                        </div>
                      ) : (
                        <div className="text-[14px] text-text-2">No savings</div>
                      )}
                    </div>

                    <div className="text-[13px] text-text-2 mb-3">
                      <p className={`m-0 ${isExpanded ? '' : 'line-clamp-3'}`}>
                        {r.reasoning}
                      </p>
                      {r.reasoning && r.reasoning.length > 120 && (
                        <button
                          type="button"
                          onClick={() =>
                            setExpandedReasoning((prev) => ({
                              ...prev,
                              [r.candidate_model]: !isExpanded,
                            }))
                          }
                          className="text-[12px] text-accent hover:underline mt-1 cursor-pointer"
                        >
                          {isExpanded ? 'Less' : 'More'}
                        </button>
                      )}
                    </div>
                  </div>

                  <div className="text-[12px] text-text-3 border-t border-border pt-2 mt-2">
                    Confidence {(experiment.recommendations.find((rec) => rec.candidate_model === r.candidate_model)?.confidence ?? 0).toFixed(2)}
                  </div>
                </div>
              );
            })}
          </div>

          {/* 2. Comparison Table (Braintrust style summary table) */}
          <Panel title="Comparison" noPadding>
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse text-[13px]">
                <thead>
                  <tr className="h-8 bg-bg-subtle border-b border-border text-[12px] text-text-2 font-medium">
                    <th className="px-4 py-2 w-48">Metric</th>
                    <th className="px-4 py-2 bg-bg-muted mono-ui w-48">
                      {experiment.original_model} (baseline)
                    </th>
                    {experiment.results.map((r) => (
                      <th key={r.candidate_model} className="px-4 py-2 mono-ui">
                        {r.candidate_model}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {/* 1. Verdict */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Verdict</td>
                    <td className="px-4 bg-bg-subtle text-text-3">—</td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4">
                        <StatusLabel value={r.recommendation} />
                      </td>
                    ))}
                  </tr>

                  {/* 2. Quality */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Quality</td>
                    <td className="px-4 bg-bg-subtle tabular-nums">100.0%</td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums">
                        {formatPercent(r.avg_quality_score)}
                      </td>
                    ))}
                  </tr>

                  {/* 3-6. Judge metrics (only if judge data exists) */}
                  {hasJudgeData && (
                    <>
                      <tr className="h-9 hover:bg-bg-muted">
                        <td className="px-4 font-medium text-text">Judge score</td>
                        <td className="px-4 bg-bg-subtle text-text-3">—</td>
                        {experiment.results.map((r) => (
                          <td key={r.candidate_model} className="px-4 tabular-nums">
                            {r.judge_score !== null && r.judge_score !== undefined
                              ? r.judge_score
                              : 'Not judged'}
                          </td>
                        ))}
                      </tr>
                      <tr className="h-9 hover:bg-bg-muted">
                        <td className="px-4 font-medium text-text">Judge W / T / L</td>
                        <td className="px-4 bg-bg-subtle text-text-3">—</td>
                        {experiment.results.map((r) => (
                          <td key={r.candidate_model} className="px-4 tabular-nums">
                            {r.judge_wins ?? '—'} / {r.judge_ties ?? '—'} / {r.judge_losses ?? '—'}
                          </td>
                        ))}
                      </tr>
                      <tr className="h-9 hover:bg-bg-muted">
                        <td className="px-4 font-medium text-text">Judge coverage</td>
                        <td className="px-4 bg-bg-subtle text-text-3">—</td>
                        {experiment.results.map((r) => (
                          <td key={r.candidate_model} className="px-4 tabular-nums">
                            {r.judge_coverage !== null && r.judge_coverage !== undefined
                              ? formatPercent(r.judge_coverage)
                              : '—'}
                          </td>
                        ))}
                      </tr>
                      <tr className="h-9 hover:bg-bg-muted">
                        <td className="px-4 font-medium text-text">Judge disagreement</td>
                        <td className="px-4 bg-bg-subtle text-text-3">—</td>
                        {experiment.results.map((r) => (
                          <td key={r.candidate_model} className="px-4 tabular-nums">
                            {r.judge_disagreement_rate !== null &&
                            r.judge_disagreement_rate !== undefined
                              ? formatPercent(r.judge_disagreement_rate)
                              : '—'}
                          </td>
                        ))}
                      </tr>
                    </>
                  )}

                  {/* 7. JSON validity */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">JSON validity</td>
                    <td className="px-4 bg-bg-subtle text-text-3">—</td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums">
                        {r.json_samples === 0 ? 'n/a' : formatPercent(r.json_validity_rate)}
                      </td>
                    ))}
                  </tr>

                  {/* 8. Error rate */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Error rate</td>
                    <td className="px-4 bg-bg-subtle tabular-nums">0.0%</td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums">
                        <div>{formatPercent(r.error_rate)}</div>
                        {r.error_rate > 0 && (
                          <div className="text-[12px] text-danger">
                            +{formatPercent(r.error_rate)}
                          </div>
                        )}
                      </td>
                    ))}
                  </tr>

                  {/* 9. p50 latency */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">p50 latency</td>
                    <td className="px-4 bg-bg-subtle tabular-nums">
                      {formatDuration(experiment.results[0]?.original_avg_latency_ms)}
                    </td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums">
                        {formatDuration(r.p50_latency_ms)}
                      </td>
                    ))}
                  </tr>

                  {/* 10. p95 latency */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">p95 latency</td>
                    <td className="px-4 bg-bg-subtle tabular-nums">
                      {formatDuration(experiment.results[0]?.original_avg_latency_ms)}
                    </td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums">
                        {formatDuration(r.p95_latency_ms)}
                      </td>
                    ))}
                  </tr>

                  {/* 11. Avg latency */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Avg latency</td>
                    <td className="px-4 bg-bg-subtle tabular-nums">
                      {formatDuration(experiment.results[0]?.original_avg_latency_ms)}
                    </td>
                    {experiment.results.map((r) => {
                      const delta = r.latency_delta_percent;
                      const isBetter = delta < 0;
                      return (
                        <td key={r.candidate_model} className="px-4 tabular-nums">
                          <div>{formatDuration(r.candidate_avg_latency_ms)}</div>
                          <div
                            className={`text-[12px] ${
                              isBetter ? 'text-success' : 'text-danger'
                            }`}
                          >
                            {formatPercentDelta(delta)}
                          </div>
                        </td>
                      );
                    })}
                  </tr>

                  {/* 12. Cost */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Cost</td>
                    <td className="px-4 bg-bg-subtle tabular-nums">
                      {formatMoney(experiment.results[0]?.original_cost_dollars)}
                    </td>
                    {experiment.results.map((r) => {
                      const delta = -r.cost_savings_percent;
                      const isBetter = r.cost_savings_dollars > 0;
                      return (
                        <td key={r.candidate_model} className="px-4 tabular-nums">
                          <div>{formatMoney(r.candidate_cost_dollars)}</div>
                          {r.cost_savings_percent !== 0 && (
                            <div
                              className={`text-[12px] ${
                                isBetter ? 'text-success' : 'text-danger'
                              }`}
                            >
                              {formatPercentDelta(delta)}
                            </div>
                          )}
                        </td>
                      );
                    })}
                  </tr>

                  {/* 13. Cost savings */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Cost savings</td>
                    <td className="px-4 bg-bg-subtle text-text-3">—</td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums">
                        {formatMoney(r.cost_savings_dollars)} ({formatPercent(r.cost_savings_percent)})
                      </td>
                    ))}
                  </tr>

                  {/* 14. Savings / month */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Savings / month</td>
                    <td className="px-4 bg-bg-subtle text-text-3">—</td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums font-medium">
                        {r.projected_monthly_savings > 0
                          ? formatMoney(r.projected_monthly_savings)
                          : '—'}
                      </td>
                    ))}
                  </tr>

                  {/* 15. Successful replays */}
                  <tr className="h-9 hover:bg-bg-muted">
                    <td className="px-4 font-medium text-text">Successful replays</td>
                    <td className="px-4 bg-bg-subtle tabular-nums">
                      {experiment.total_replays} / {experiment.total_replays}
                    </td>
                    {experiment.results.map((r) => (
                      <td key={r.candidate_model} className="px-4 tabular-nums">
                        {r.successful_requests} / {r.total_requests}
                      </td>
                    ))}
                  </tr>
                </tbody>
              </table>
            </div>
          </Panel>

          {/* 3. Charts Row: 2 columns 50/50, 16px gap */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {/* Cost per candidate */}
            <ChartContainer title="Cost per candidate" timeRangeText="USD">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart
                  data={costChartData}
                  layout="vertical"
                  margin={{ top: 10, right: 20, left: 40, bottom: 0 }}
                >
                  <XAxis
                    type="number"
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 12, fill: 'var(--text-3)' }}
                    tickFormatter={(v) => `$${v}`}
                  />
                  <YAxis
                    dataKey="name"
                    type="category"
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 12, fill: 'var(--text-3)' }}
                    width={120}
                  />
                  <RechartsTooltip content={<CustomChartTooltip />} />
                  <Bar
                    dataKey="cost"
                    radius={[0, 2, 2, 0]}
                    maxBarSize={20}
                    fill="var(--chart-1)"
                  />
                </BarChart>
              </ResponsiveContainer>
            </ChartContainer>

            {/* Latency p50 and p95 */}
            <ChartContainer title="Latency p50 and p95" timeRangeText="ms">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart
                  data={latencyChartData}
                  margin={{ top: 10, right: 20, left: 0, bottom: 0 }}
                >
                  <XAxis
                    dataKey="name"
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 12, fill: 'var(--text-3)' }}
                  />
                  <YAxis
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 12, fill: 'var(--text-3)' }}
                    tickCount={4}
                  />
                  <RechartsTooltip content={<CustomChartTooltip />} />
                  <Bar
                    name="p50"
                    dataKey="p50"
                    fill="var(--chart-1)"
                    radius={[2, 2, 0, 0]}
                    maxBarSize={16}
                  />
                  <Bar
                    name="p95"
                    dataKey="p95"
                    fill="var(--chart-2)"
                    radius={[2, 2, 0, 0]}
                    maxBarSize={16}
                  />
                </BarChart>
              </ResponsiveContainer>
            </ChartContainer>
          </div>

          {/* 4. Judge Samples (only when judge data exists) */}
          {hasJudgeData && (
            <Panel title="Judge samples">
              <Tabs
                tabs={experiment.results
                  .filter((r) => r.judge_samples && r.judge_samples.length > 0)
                  .map((r) => ({
                    value: r.candidate_model,
                    label: <span className="mono-ui">{r.candidate_model}</span>,
                    content: (
                      <div className="space-y-3">
                        {r.judge_samples?.map((sample, sIdx) => {
                          const sampleKey = `${r.candidate_model}-${sIdx}`;
                          const isSampleOpen = expandedSamples[sampleKey] || false;

                          let scoreLabel = 'Split';
                          let scoreTone: 'success' | 'warning' | 'danger' = 'warning';
                          if (sample.judge_score >= 100) {
                            scoreLabel = 'Candidate better or equal';
                            scoreTone = 'success';
                          } else if (sample.judge_score === 0) {
                            scoreLabel = 'Baseline better';
                            scoreTone = 'danger';
                          }

                          return (
                            <div
                              key={sIdx}
                              className="border border-border rounded-md overflow-hidden bg-bg"
                            >
                              <div
                                onClick={() =>
                                  setExpandedSamples((prev) => ({
                                    ...prev,
                                    [sampleKey]: !isSampleOpen,
                                  }))
                                }
                                className="h-10 px-4 flex items-center justify-between cursor-pointer hover:bg-bg-muted"
                              >
                                <div className="flex items-center gap-2 truncate pr-4">
                                  <ChevronRightIcon
                                    className={`w-4 h-4 text-text-2 shrink-0 transition-transform ${
                                      isSampleOpen ? 'rotate-90' : ''
                                    }`}
                                  />
                                  <span className="text-[13px] text-text truncate">
                                    {sample.prompt_excerpt}
                                  </span>
                                </div>
                                <StatusLabel
                                  value={
                                    scoreTone === 'success'
                                      ? 'SAFE_TO_SWITCH'
                                      : scoreTone === 'warning'
                                      ? 'SWITCH_WITH_CAUTION'
                                      : 'DO_NOT_SWITCH'
                                  }
                                  customText={scoreLabel}
                                />
                              </div>

                              {isSampleOpen && (
                                <div className="p-4 border-t border-border bg-bg-subtle space-y-4">
                                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                                    <div className="bg-bg p-3 rounded-md border border-border">
                                      <div className="text-[12px] font-medium text-text-2 mb-1.5">
                                        Baseline excerpt
                                      </div>
                                      <div className="text-[13px] text-text leading-5">
                                        {sample.baseline_excerpt}
                                      </div>
                                    </div>
                                    <div className="bg-bg p-3 rounded-md border border-border">
                                      <div className="text-[12px] font-medium text-text-2 mb-1.5">
                                        Candidate excerpt
                                      </div>
                                      <div className="text-[13px] text-text leading-5">
                                        {sample.candidate_excerpt}
                                      </div>
                                    </div>
                                  </div>

                                  {sample.orders && sample.orders.length > 0 && (
                                    <div className="space-y-1 text-[12px] text-text-2">
                                      {sample.orders.map((ord, oIdx) => (
                                        <div key={oIdx}>
                                          Baseline as {ord.baseline_position || 'A'} → winner{' '}
                                          {ord.winner || 'tie'} · {ord.reason || 'No comment.'}
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    ),
                  }))}
              />
            </Panel>
          )}
        </div>
      )}
    </div>
  );
};

