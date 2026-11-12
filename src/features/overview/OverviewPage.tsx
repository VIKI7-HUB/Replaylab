import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  LineChart,
  Line,
  XAxis,
  YAxis,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
} from 'recharts';
import { PageHeader } from '../../app/PageHeader';
import { Select } from '../../components/Select';
import { Button } from '../../components/Button';
import { Panel } from '../../components/Panel';
import { StatStrip } from '../../components/StatStrip';
import { StatusLabel } from '../../components/StatusLabel';
import { ChartContainer, CustomChartTooltip } from '../../components/Chart';
import { AnalyticsOfflineState, Skeleton } from '../../components/States';
import { FreshnessMeter } from '../../components/Meter';
import {
  getOverview,
  getReplayLabDaily,
  getReplayLabVerdicts,
  getStaleDaily,
  InsightsOverview,
  DailyReplayLabPoint,
  VerdictPoint,
  StaleDailyPoint,
} from '../../api/insights';
import {
  formatCount,
  formatMoney,
  formatPercent,
  formatRelativeTime,
} from '../../lib/format';

export const OverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [range, setRange] = useState('30');
  const [loading, setLoading] = useState(true);
  const [offline, setOffline] = useState(false);

  const [overview, setOverview] = useState<InsightsOverview | null>(null);
  const [replayDaily, setReplayDaily] = useState<DailyReplayLabPoint[]>([]);
  const [verdicts, setVerdicts] = useState<VerdictPoint[]>([]);
  const [staleDaily, setStaleDaily] = useState<StaleDailyPoint[]>([]);

  const fetchOverviewData = async () => {
    setLoading(true);
    setOffline(false);
    try {
      const days = parseInt(range, 10) || 30;
      const [ovData, rDaily, vData, sDaily] = await Promise.all([
        getOverview(days),
        getReplayLabDaily(days),
        getReplayLabVerdicts(90),
        getStaleDaily(days),
      ]);
      setOverview(ovData);
      setReplayDaily(rDaily);
      setVerdicts(vData);
      setStaleDaily(sDaily);
    } catch {
      setOffline(true);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchOverviewData();
  }, [range]);

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton height="h-8" width="w-48" />
        <Skeleton height="h-4" width="w-96" />
        <Skeleton height="h-28" />
        <Skeleton height="h-28" />
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <Skeleton height="h-64" />
          <Skeleton height="h-64" />
        </div>
      </div>
    );
  }

  // §13: If the Insights API is down, the whole page shows the Analytics offline state, plus two buttons: [Go to captures] and [Check an answer].
  if (offline || !overview) {
    return (
      <div>
        <PageHeader
          title="Overview"
          description="Traffic, experiments and answer freshness across both services."
        />
        <Panel>
          <AnalyticsOfflineState
            onRetry={fetchOverviewData}
            extraActions={
              <div className="flex items-center gap-2">
                <Button size="sm" variant="secondary" onClick={() => navigate('/replaylab/captures')}>
                  Go to captures
                </Button>
                <Button size="sm" variant="secondary" onClick={() => navigate('/stale/check')}>
                  Check an answer
                </Button>
              </div>
            }
          />
        </Panel>
      </div>
    );
  }

  // Prepare chart 1 data: "Captures per day by model" (stacked area)
  // Group by date and spread models
  const captureChartMap = new Map<string, any>();
  const modelsSet = new Set<string>();
  replayDaily.forEach((pt) => {
    modelsSet.add(pt.model);
    if (!captureChartMap.has(pt.date)) {
      captureChartMap.set(pt.date, { date: pt.date });
    }
    const cur = captureChartMap.get(pt.date);
    cur[pt.model] = (cur[pt.model] || 0) + pt.captures;
  });
  const captureChartData = Array.from(captureChartMap.values());
  const modelsList = Array.from(modelsSet);

  // Prepare chart 2 data: "Cost per day by model" (stacked bars)
  const costChartMap = new Map<string, any>();
  replayDaily.forEach((pt) => {
    if (!costChartMap.has(pt.date)) {
      costChartMap.set(pt.date, { date: pt.date });
    }
    const cur = costChartMap.get(pt.date);
    cur[pt.model] = (cur[pt.model] || 0) + pt.cost_usd;
  });
  const costChartData = Array.from(costChartMap.values());

  // Prepare chart 4 data: "Freshness by claim type" (one line per claim type)
  const staleChartMap = new Map<string, any>();
  const claimTypesSet = new Set<string>();
  staleDaily.forEach((pt) => {
    claimTypesSet.add(pt.claim_type);
    if (!staleChartMap.has(pt.date)) {
      staleChartMap.set(pt.date, { date: pt.date });
    }
    staleChartMap.get(pt.date)[pt.claim_type] = pt.avg_freshness;
  });
  const staleChartData = Array.from(staleChartMap.values());
  const claimTypesList = Array.from(claimTypesSet);

  return (
    <div className="space-y-6">
      {/* Header */}
      <PageHeader
        title="Overview"
        description="Traffic, experiments and answer freshness across both services."
        actions={
          <div className="w-36">
            <Select
              value={range}
              onValueChange={setRange}
              options={[
                { value: '1', label: 'Last 24 hours' },
                { value: '7', label: '7 days' },
                { value: '30', label: '30 days' },
              ]}
            />
          </div>
        }
      />

      {/* Stat Strip 1: Replay Lab */}
      <StatStrip
        cells={[
          {
            label: 'Captures (24 h)',
            value: formatCount(overview.replaylab.captures_24h),
            detail: 'recorded traffic',
          },
          {
            label: 'Cost (24 h)',
            value: formatMoney(overview.replaylab.cost_24h_usd),
            detail: 'actual spend',
          },
          {
            label: 'Error rate (24 h)',
            value: formatPercent(overview.replaylab.error_rate_24h),
            detail: 'upstream failures',
          },
          {
            label: 'Savings available',
            value: formatMoney(overview.replaylab.safe_switch_monthly_savings_usd),
            detail: 'per month from safe switches',
          },
        ]}
      />

      {/* Stat Strip 2: Stale answers */}
      <StatStrip
        cells={[
          {
            label: 'Reports (7 d)',
            value: formatCount(overview.stale.reports_7d),
            detail: 'answers audited',
          },
          {
            label: 'Average freshness (7 d)',
            value: overview.stale.avg_freshness_7d.toFixed(1),
            detail: '0-100, higher is fresher',
          },
          {
            label: 'Stale claims (7 d)',
            value: formatCount(overview.stale.stale_claims_7d),
            detail: 'outdated facts found',
          },
          {
            label: 'Verification mode',
            value: `${Math.round(overview.stale.llm_mode_percent)}% LLM`,
            detail: `${Math.round(overview.stale.heuristic_mode_percent)}% heuristic`,
          },
        ]}
      />

      {/* Charts Grid: 2 columns, 16px gap */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* 1. Captures per day by model (stacked area, flat fill) */}
        <ChartContainer title="Captures per day by model" timeRangeText="Last 30 days">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={captureChartData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
              <XAxis dataKey="date" axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} />
              <YAxis axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} tickCount={4} />
              <RechartsTooltip content={<CustomChartTooltip />} />
              {modelsList.map((model, idx) => (
                <Area
                  key={model}
                  type="monotone"
                  dataKey={model}
                  stackId="1"
                  strokeWidth={1.5}
                  stroke={`var(--chart-${(idx % 6) + 1})`}
                  fill={`var(--chart-${(idx % 6) + 1})`}
                  fillOpacity={0.12}
                  dot={false}
                  activeDot={{ r: 3 }}
                />
              ))}
            </AreaChart>
          </ResponsiveContainer>
        </ChartContainer>

        {/* 2. Cost per day by model (stacked bars) */}
        <ChartContainer title="Cost per day by model" timeRangeText="USD">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={costChartData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
              <XAxis dataKey="date" axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} />
              <YAxis axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} tickCount={4} tickFormatter={(v) => `$${v}`} />
              <RechartsTooltip content={<CustomChartTooltip />} />
              {modelsList.map((model, idx) => (
                <Bar
                  key={model}
                  dataKey={model}
                  stackId="cost"
                  fill={`var(--chart-${(idx % 6) + 1})`}
                  radius={[2, 2, 0, 0]}
                  maxBarSize={24}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        </ChartContainer>

        {/* 3. Verdicts over time (stacked bars: success, warning and danger colors) */}
        <ChartContainer title="Verdicts over time" timeRangeText="Last 90 days">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={verdicts} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
              <XAxis dataKey="date" axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} />
              <YAxis axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} tickCount={4} />
              <RechartsTooltip content={<CustomChartTooltip />} />
              <Bar name="Safe to switch" dataKey="safe" stackId="verdicts" fill="var(--success)" radius={[0, 0, 0, 0]} maxBarSize={24} />
              <Bar name="Caution" dataKey="caution" stackId="verdicts" fill="var(--warning)" radius={[0, 0, 0, 0]} maxBarSize={24} />
              <Bar name="Do not switch" dataKey="do_not_switch" stackId="verdicts" fill="var(--danger)" radius={[2, 2, 0, 0]} maxBarSize={24} />
            </BarChart>
          </ResponsiveContainer>
        </ChartContainer>

        {/* 4. Freshness by claim type (one line per claim type) */}
        <ChartContainer title="Freshness by claim type" timeRangeText="Score 0-100">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={staleChartData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
              <XAxis dataKey="date" axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} />
              <YAxis domain={[0, 100]} axisLine={false} tickLine={false} tick={{ fontSize: 12, fill: 'var(--text-3)' }} tickCount={4} />
              <RechartsTooltip content={<CustomChartTooltip />} />
              {claimTypesList.map((type, idx) => (
                <Line
                  key={type}
                  type="monotone"
                  dataKey={type}
                  strokeWidth={1.5}
                  stroke={`var(--chart-${(idx % 6) + 1})`}
                  dot={false}
                  activeDot={{ r: 3 }}
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        </ChartContainer>
      </div>

      {/* Recent Activity: 2 columns */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Latest experiments */}
        <Panel
          title="Latest experiments"
          actions={
            <Button size="sm" variant="ghost" onClick={() => navigate('/replaylab/experiments')}>
              View all
            </Button>
          }
          noPadding
        >
          <div className="divide-y divide-border">
            {overview.latest_experiments.length === 0 ? (
              <div className="p-4 text-[13px] text-text-3">No recent experiments</div>
            ) : (
              overview.latest_experiments.slice(0, 5).map((exp) => (
                <div
                  key={exp.id}
                  onClick={() => navigate(`/replaylab/experiments/${encodeURIComponent(exp.id)}`)}
                  className="h-10 px-4 flex items-center justify-between text-[13px] hover:bg-bg-muted cursor-pointer transition-colors"
                >
                  <div className="flex items-center gap-2 truncate pr-2">
                    <span className="font-medium text-text truncate">{exp.name}</span>
                    <StatusLabel value={exp.status} />
                  </div>
                  <div className="flex items-center gap-3 shrink-0">
                    {exp.best_verdict && (
                      <div className="inline-flex items-center gap-1">
                        <StatusLabel value={exp.best_verdict} />
                        {exp.best_verdict_model && (
                          <span className="text-[12px] text-text-2 truncate max-w-[80px]">
                            {exp.best_verdict_model}
                          </span>
                        )}
                      </div>
                    )}
                    <span className="text-[12px] text-text-3">
                      {formatRelativeTime(exp.created_at)}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
        </Panel>

        {/* Latest reports */}
        <Panel
          title="Latest reports"
          actions={
            <Button size="sm" variant="ghost" onClick={() => navigate('/stale/history')}>
              View all
            </Button>
          }
          noPadding
        >
          <div className="divide-y divide-border">
            {overview.latest_reports.length === 0 ? (
              <div className="p-4 text-[13px] text-text-3">No recent reports</div>
            ) : (
              overview.latest_reports.slice(0, 5).map((rep) => (
                <div
                  key={rep.id}
                  onClick={() => navigate(`/stale/reports/${encodeURIComponent(rep.id)}`)}
                  className="h-10 px-4 flex items-center justify-between text-[13px] hover:bg-bg-muted cursor-pointer transition-colors"
                >
                  <span className="text-text truncate max-w-[200px] md:max-w-[280px]">
                    {rep.text_preview}
                  </span>
                  <div className="flex items-center gap-3 shrink-0">
                    <FreshnessMeter score={rep.freshness_score} />
                    <span
                      className={`text-[12px] font-medium tabular-nums ${
                        rep.stale_count > 0 ? 'text-danger' : 'text-text-3'
                      }`}
                    >
                      {rep.stale_count} stale
                    </span>
                    <span className="text-[12px] text-text-3">
                      {formatRelativeTime(rep.checked_at)}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
        </Panel>
      </div>
    </div>
  );
};

