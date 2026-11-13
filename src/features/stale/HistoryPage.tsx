import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ColumnDef } from '@tanstack/react-table';
import { PageHeader } from '../../app/PageHeader';
import { Input } from '../../components/Input';
import { Panel } from '../../components/Panel';
import { DataTable, TruncatedCell } from '../../components/DataTable';
import { FreshnessMeter } from '../../components/Meter';
import { EmptyState, ErrorState, TableSkeleton } from '../../components/States';
import { Tooltip } from '../../components/Tooltip';
import { HistoryItem, getHistory } from '../../api/stale';
import { searchReports } from '../../api/insights';
import { insightsHealth } from '../../api/health';
import { errorMessage } from '../../api/error';
import { formatIsoUtc, formatRelativeTime } from '../../lib/format';

export const HistoryPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const page = parseInt(searchParams.get('page') || '0', 10);

  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Insights search input visibility & state
  const [insightsAvailable, setInsightsAvailable] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  useEffect(() => {
    insightsHealth().then((h) => setInsightsAvailable(h.status === 'healthy'));
  }, []);

  const fetchHistory = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getHistory(20, page * 20);
      setHistory(data);
    } catch (err) {
      setError(errorMessage(err, "Couldn't load history"));
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchHistory();
  }, [fetchHistory]);

  // A search query asks the read API (§14.3); without one the table shows the service's history.
  const [searchResults, setSearchResults] = useState<HistoryItem[] | null>(null);

  useEffect(() => {
    const q = searchQuery.trim();
    if (!q) {
      setSearchResults(null);
      return;
    }
    const timer = setTimeout(async () => {
      try {
        setSearchResults(await searchReports(q));
        setError(null);
      } catch (err) {
        setError(errorMessage(err, "Couldn't search reports"));
      }
    }, 200);
    return () => clearTimeout(timer);
  }, [searchQuery]);

  const filteredHistory = useMemo(() => searchResults ?? history, [searchResults, history]);

  // Table columns strictly in order: Checked, Question, Freshness, Stale, Current
  const columns = useMemo<ColumnDef<HistoryItem>[]>(
    () => [
      {
        accessorKey: 'checked_at',
        header: 'Checked',
        cell: ({ getValue }) => {
          const val = getValue<string>();
          return (
            <Tooltip content={formatIsoUtc(val)}>
              <span className="mono-ui text-[12px] text-text-2">
                {formatRelativeTime(val)}
              </span>
            </Tooltip>
          );
        },
      },
      {
        accessorKey: 'question',
        header: 'Question',
        cell: ({ getValue }) => (
          <TruncatedCell text={getValue<string>() || '—'} maxWidth="max-w-md" />
        ),
      },
      {
        accessorKey: 'overall_freshness_score',
        header: 'Freshness',
        cell: ({ getValue }) => (
          <FreshnessMeter score={getValue<number>()} />
        ),
      },
      {
        accessorKey: 'stale_count',
        header: 'Stale',
        meta: { isNumeric: true },
        cell: ({ getValue }) => {
          const count = getValue<number>();
          return (
            <span
              className={`tabular-nums font-medium ${
                count > 0 ? 'text-danger' : 'text-text'
              }`}
            >
              {count}
            </span>
          );
        },
      },
      {
        accessorKey: 'current_count',
        header: 'Current',
        meta: { isNumeric: true },
        cell: ({ getValue }) => (
          <span className="tabular-nums">{getValue<number>()}</span>
        ),
      },
    ],
    []
  );

  return (
    <div>
      <PageHeader
        title="History"
        description="Every answer checked, newest first."
      />

      {/* Insights search input when Insights API is up */}
      {insightsAvailable && (
        <div className="mb-4 max-w-sm">
          <Input
            placeholder="Search reports…"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      )}

      <Panel noPadding>
        {loading ? (
          <TableSkeleton rows={6} />
        ) : error ? (
          <ErrorState
            title="Couldn't load history"
            message={error}
            onRetry={fetchHistory}
          />
        ) : filteredHistory.length === 0 ? (
          <EmptyState
            title="No checks yet"
            description="Run a check on an answer to see it recorded here."
            actionLabel="Check an answer"
            onAction={() => navigate('/stale/check')}
          />
        ) : (
          <DataTable
            columns={columns}
            data={filteredHistory}
            getRowId={(item) => item.report_id}
            onRowClick={(item) =>
              navigate(`/stale/reports/${encodeURIComponent(item.report_id)}`)
            }
            pageIndex={page}
            pageSize={20}
            totalCount={
              filteredHistory.length >= 20 ? (page + 2) * 20 : page * 20 + filteredHistory.length
            }
            onPageChange={(p) => {
              const next = new URLSearchParams(searchParams);
              next.set('page', String(p));
              setSearchParams(next);
            }}
          />
        )}
      </Panel>
    </div>
  );
};

