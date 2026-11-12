import React, { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { ColumnDef } from '@tanstack/react-table';
import { PageHeader } from '../../app/PageHeader';
import { Button } from '../../components/Button';
import { Panel } from '../../components/Panel';
import { DataTable } from '../../components/DataTable';
import { StatusLabel } from '../../components/StatusLabel';
import { EmptyState, ErrorState, TableSkeleton } from '../../components/States';
import { Tooltip } from '../../components/Tooltip';
import { Experiment, getExperiments } from '../../api/replaylab';
import { formatIsoUtc, formatMoney, formatRelativeTime } from '../../lib/format';

export const ExperimentsListPage: React.FC = () => {
  const navigate = useNavigate();
  const [experiments, setExperiments] = useState<Experiment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchExperiments = async (isBackground = false) => {
    if (!isBackground) setLoading(true);
    setError(null);
    try {
      const data = await getExperiments(50);
      setExperiments(data);
    } catch (err: any) {
      if (!isBackground) {
        setError(err.message || 'Failed to load experiments');
      }
    } finally {
      if (!isBackground) setLoading(false);
    }
  };

  useEffect(() => {
    fetchExperiments();
  }, []);

  // Poll every 5s while any experiment is running (§15)
  useEffect(() => {
    const hasRunning = experiments.some((exp) => exp.status === 'running');
    if (!hasRunning) return;

    const interval = setInterval(() => {
      fetchExperiments(true);
    }, 5000);
    return () => clearInterval(interval);
  }, [experiments]);

  // Helper to determine best verdict among results
  // SAFE_TO_SWITCH > SWITCH_WITH_CAUTION > DO_NOT_SWITCH
  const getBestVerdict = (exp: Experiment) => {
    if (!exp.results || exp.results.length === 0) return null;

    const safe = exp.results.find((r) => r.recommendation === 'SAFE_TO_SWITCH');
    if (safe) return { verdict: safe.recommendation, model: safe.candidate_model };

    const caution = exp.results.find((r) => r.recommendation === 'SWITCH_WITH_CAUTION');
    if (caution) return { verdict: caution.recommendation, model: caution.candidate_model };

    const doNot = exp.results.find((r) => r.recommendation === 'DO_NOT_SWITCH');
    if (doNot) return { verdict: doNot.recommendation, model: doNot.candidate_model };

    return null;
  };

  // Helper to get largest projected monthly savings among SAFE_TO_SWITCH candidates
  const getMaxSafeSavings = (exp: Experiment) => {
    if (!exp.results) return null;
    const safeCandidates = exp.results.filter(
      (r) => r.recommendation === 'SAFE_TO_SWITCH' && r.projected_monthly_savings > 0
    );
    if (safeCandidates.length === 0) return null;
    return Math.max(...safeCandidates.map((r) => r.projected_monthly_savings));
  };

  const columns = useMemo<ColumnDef<Experiment>[]>(
    () => [
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ getValue }) => (
          <span className="font-medium text-text">{getValue<string>()}</span>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => (
          <StatusLabel value={row.original.status} stopReason={row.original.stop_reason} />
        ),
      },
      {
        accessorKey: 'original_model',
        header: 'Baseline',
        cell: ({ getValue }) => (
          <span className="mono-ui text-[12.5px] text-text">
            {getValue<string>()}
          </span>
        ),
      },
      {
        id: 'candidates',
        header: 'Candidates',
        cell: ({ row }) => {
          const c = row.original.candidate_models || [];
          if (c.length === 0) return <span>—</span>;
          const display = c.slice(0, 2).join(', ');
          const extra = c.length > 2 ? ` +${c.length - 2}` : '';
          return (
            <span className="mono-ui text-[12.5px] text-text-2">
              {display}{extra}
            </span>
          );
        },
      },
      {
        id: 'best_verdict',
        header: 'Best verdict',
        cell: ({ row }) => {
          const best = getBestVerdict(row.original);
          if (!best) return <span className="text-text-3">—</span>;
          return (
            <div className="inline-flex items-center gap-1.5">
              <StatusLabel value={best.verdict} />
              <span className="text-[12px] text-text-2 truncate max-w-[100px]">
                {best.model}
              </span>
            </div>
          );
        },
      },
      {
        id: 'savings',
        header: 'Savings / month',
        meta: { isNumeric: true },
        cell: ({ row }) => {
          const maxSavings = getMaxSafeSavings(row.original);
          if (maxSavings === null) return <span className="text-text-3">—</span>;
          return <span className="tabular-nums font-medium">{formatMoney(maxSavings)}</span>;
        },
      },
      {
        id: 'spent',
        header: 'Spent',
        cell: ({ row }) => (
          <span className="text-[12px] text-text-2 tabular-nums">
            {formatMoney(row.original.spent_dollars)} of {formatMoney(row.original.budget_cap_dollars)}
          </span>
        ),
      },
      {
        id: 'progress',
        header: 'Progress',
        cell: ({ row }) => {
          const exp = row.original;
          if (exp.status === 'running') {
            const pct = exp.total_replays > 0
              ? Math.min(100, Math.round((exp.completed_replays / exp.total_replays) * 100))
              : 0;
            return (
              <div className="flex items-center gap-2">
                <div className="w-16 h-1 bg-bg-muted rounded-none overflow-hidden shrink-0">
                  <div
                    className="h-full bg-text transition-all duration-150"
                    style={{ width: `${pct}%` }}
                  />
                </div>
                <span className="text-[12px] text-text-2 tabular-nums">{pct}%</span>
              </div>
            );
          }
          return (
            <span className="text-[12px] text-text-2 tabular-nums">
              {exp.completed_replays} / {exp.total_replays}
            </span>
          );
        },
      },
      {
        accessorKey: 'created_at',
        header: 'Created',
        cell: ({ getValue }) => (
          <Tooltip content={formatIsoUtc(getValue<string>())}>
            <span className="text-[12px] text-text-2">
              {formatRelativeTime(getValue<string>())}
            </span>
          </Tooltip>
        ),
      },
    ],
    []
  );

  return (
    <div>
      <PageHeader
        title="Experiments"
        description="Replay captured traffic against cheaper models and get a verdict per model."
        actions={
          <Button
            size="md"
            variant="primary"
            onClick={() => navigate('/replaylab/experiments/new')}
          >
            New experiment
          </Button>
        }
      />

      <Panel noPadding>
        {loading ? (
          <TableSkeleton rows={6} />
        ) : error ? (
          <ErrorState
            title="Couldn't load experiments"
            message={error}
            onRetry={() => fetchExperiments()}
          />
        ) : experiments.length === 0 ? (
          <EmptyState
            title="No experiments yet"
            description="Create an experiment to test cheaper models against your captured traffic."
            actionLabel="New experiment"
            onAction={() => navigate('/replaylab/experiments/new')}
          />
        ) : (
          <DataTable
            columns={columns}
            data={experiments}
            getRowId={(e) => e.id}
            onRowClick={(e) => navigate(`/replaylab/experiments/${encodeURIComponent(e.id)}`)}
          />
        )}
      </Panel>
    </div>
  );
};

