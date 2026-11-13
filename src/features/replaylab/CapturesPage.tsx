import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { useSearchParams, useNavigate, useParams } from 'react-router-dom';
import { ColumnDef } from '@tanstack/react-table';
import { PageHeader } from '../../app/PageHeader';
import { Button } from '../../components/Button';
import { Input } from '../../components/Input';
import { Select } from '../../components/Select';
import { Panel } from '../../components/Panel';
import { DataTable, TruncatedCell } from '../../components/DataTable';
import { StatusLabel } from '../../components/StatusLabel';
import { CaptureDrawer } from './CaptureDrawer';
import { EmptyState, ErrorState, TableSkeleton } from '../../components/States';
import { Tooltip } from '../../components/Tooltip';
import {
  Capture,
  getCaptures,
  getCaptureById,
  getModels,
  ModelPrice,
  sendSampleTraffic,
  SAMPLE_TRAFFIC_COUNT,
} from '../../api/replaylab';
import { useToast } from '../../components/Toast';
import {
  formatDuration,
  formatIsoUtc,
  formatMoney,
  formatRelativeTime,
  formatTokens,
} from '../../lib/format';
import { errorMessage } from '../../api/error';

export const CapturesPage: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const { id: paramId } = useParams<{ id?: string }>();

  // Filter states stored in searchParams
  const modelFilter = searchParams.get('model') || '';
  const tagFilter = searchParams.get('tag') || '';
  const virtualKeyFilter = searchParams.get('virtual_key') || '';
  const streamedFilter = searchParams.get('streamed') || '';
  const page = parseInt(searchParams.get('page') || '0', 10);

  const [models, setModels] = useState<ModelPrice[]>([]);
  const [captures, setCaptures] = useState<Capture[]>([]);
  const [selectedCapture, setSelectedCapture] = useState<Capture | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { toast } = useToast();

  // Sample traffic: null when idle, else how many requests have been sent.
  const [sampleSent, setSampleSent] = useState<number | null>(null);

  // Load models for filter
  useEffect(() => {
    getModels()
      .then((m) => setModels(m))
      .catch(() => {});
  }, []);

  const fetchCaptures = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getCaptures({
        model: modelFilter || undefined,
        tag: tagFilter || undefined,
        virtual_key: virtualKeyFilter || undefined,
        streamed:
          streamedFilter === 'true' ? true : streamedFilter === 'false' ? false : undefined,
        limit: 50,
        offset: page * 50,
      });
      setCaptures(data);
    } catch (err) {
      setError(errorMessage(err, 'Failed to load captures'));
    } finally {
      setLoading(false);
    }
  }, [modelFilter, tagFilter, virtualKeyFilter, streamedFilter, page]);

  useEffect(() => {
    fetchCaptures();
  }, [fetchCaptures]);

  // Handle drawer selection
  useEffect(() => {
    if (paramId) {
      const found = captures.find((c) => c.id === paramId);
      if (found) {
        setSelectedCapture(found);
      } else {
        getCaptureById(paramId)
          .then((c) => setSelectedCapture(c))
          .catch(() => setSelectedCapture(null));
      }
    } else {
      setSelectedCapture(null);
    }
  }, [paramId, captures]);

  // With nothing captured yet, a visitor can send real requests through the
  // gateway from here. They go to the priciest model that answers, so an
  // experiment has cheaper candidates to try.
  const handleSendSample = async () => {
    const ranked = [...models]
      .sort((a, b) => b.output_per_1m_tokens - a.output_per_1m_tokens)
      .map((m) => m.model);
    if (ranked.length === 0) {
      toast('The gateway lists no models to send traffic to.', 'warning');
      return;
    }
    setSampleSent(0);
    try {
      const { model, answered } = await sendSampleTraffic(ranked, setSampleSent);
      if (!model) {
        toast('No model answered. The provider may be overloaded; try again in a minute.', 'danger');
      } else {
        toast(`${answered} of ${SAMPLE_TRAFFIC_COUNT} sample requests answered by ${model}`, answered > 0 ? 'success' : 'danger');
      }
      // Captures are written in the background a moment after each response.
      await new Promise((resolve) => setTimeout(resolve, 800));
      await fetchCaptures();
    } catch (err) {
      toast(errorMessage(err, "Couldn't send sample traffic"), 'danger');
    } finally {
      setSampleSent(null);
    }
  };

  const updateFilter = (key: string, val: string) => {
    const next = new URLSearchParams(searchParams);
    if (val) next.set(key, val);
    else next.delete(key);
    next.set('page', '0'); // reset page on filter change
    setSearchParams(next);
  };

  const clearFilters = () => {
    const next = new URLSearchParams();
    setSearchParams(next);
  };

  const hasActiveFilters = Boolean(
    modelFilter || tagFilter || virtualKeyFilter || streamedFilter
  );

  const openDrawer = (c: Capture) => {
    const next = new URLSearchParams(searchParams);
    navigate(`/replaylab/captures/${encodeURIComponent(c.id)}?${next.toString()}`);
  };

  const closeDrawer = () => {
    const next = new URLSearchParams(searchParams);
    navigate(`/replaylab/captures?${next.toString()}`);
  };

  // Keyboard navigation for drawer
  const currentIndex = selectedCapture
    ? captures.findIndex((c) => c.id === selectedCapture.id)
    : -1;

  const navigatePrev = () => {
    if (currentIndex > 0) {
      openDrawer(captures[currentIndex - 1]);
    }
  };

  const navigateNext = () => {
    if (currentIndex >= 0 && currentIndex < captures.length - 1) {
      openDrawer(captures[currentIndex + 1]);
    }
  };

  const modelOptions = [
    { value: '', label: 'All models' },
    ...models.map((m) => ({ value: m.model, label: m.model })),
  ];

  const streamedOptions = [
    { value: '', label: 'All' },
    { value: 'true', label: 'Streamed' },
    { value: 'false', label: 'Not streamed' },
  ];

  // Table columns strictly in order from §12.1:
  // 1. Time, 2. Model, 3. Tag, 4. Status, 5. State, 6. Tokens, 7. Latency, 8. TTFT, 9. Cost, 10. Streamed
  const columns = useMemo<ColumnDef<Capture>[]>(
    () => [
      {
        id: 'time',
        header: 'Time',
        cell: ({ row }) => (
          <Tooltip content={formatIsoUtc(row.original.timestamp)}>
            <span className="mono-ui text-[12px] text-text-2">
              {formatRelativeTime(row.original.timestamp)}
            </span>
          </Tooltip>
        ),
      },
      {
        accessorKey: 'model',
        header: 'Model',
        cell: ({ getValue }) => (
          <span className="mono-ui text-[12.5px] text-text">
            {getValue<string>()}
          </span>
        ),
      },
      {
        accessorKey: 'tag',
        header: 'Tag',
        cell: ({ getValue }) => (
          <TruncatedCell text={getValue<string>() || '—'} maxWidth="max-w-[120px]" />
        ),
      },
      {
        accessorKey: 'status_code',
        header: 'Status',
        cell: ({ getValue }) => {
          const code = getValue<number>();
          return (
            <span className={`text-[13px] ${code >= 400 ? 'text-danger font-medium' : 'text-text'}`}>
              {code}
            </span>
          );
        },
      },
      {
        accessorKey: 'completion_state',
        header: 'State',
        cell: ({ getValue }) => {
          const state = getValue<string>();
          if (state === 'complete') return null;
          return <StatusLabel value={state} />;
        },
      },
      {
        accessorKey: 'total_tokens',
        header: 'Tokens',
        meta: { isNumeric: true },
        cell: ({ row }) => {
          const prefix = row.original.usage_estimated ? '≈ ' : '';
          return (
            <span className="tabular-nums">
              {prefix}{formatTokens(row.original.total_tokens)}
            </span>
          );
        },
      },
      {
        accessorKey: 'latency_ms',
        header: 'Latency',
        meta: { isNumeric: true },
        cell: ({ getValue }) => (
          <span className="tabular-nums">{formatDuration(getValue<number>())}</span>
        ),
      },
      {
        accessorKey: 'time_to_first_token_ms',
        header: 'TTFT',
        meta: { isNumeric: true },
        cell: ({ getValue }) => {
          const ttft = getValue<number | null | undefined>();
          return <span className="tabular-nums">{ttft ? formatDuration(ttft) : '—'}</span>;
        },
      },
      {
        accessorKey: 'cost_estimate',
        header: 'Cost',
        meta: { isNumeric: true },
        cell: ({ row }) => {
          const prefix = row.original.usage_estimated ? '≈ ' : '';
          return (
            <span className="tabular-nums">
              {prefix}{formatMoney(row.original.cost_estimate)}
            </span>
          );
        },
      },
      {
        accessorKey: 'is_streaming',
        header: 'Streamed',
        cell: ({ getValue }) => (getValue<boolean>() ? 'Yes' : ''),
      },
    ],
    []
  );

  return (
    <div>
      {/* Header */}
      <PageHeader
        title="Captures"
        description="Every chat request recorded through the gateway, newest first."
        actions={
          <Button size="md" variant="secondary" onClick={fetchCaptures}>
            Refresh
          </Button>
        }
      />

      {/* Filter bar: 16px below header, 8px gaps */}
      <div className="flex flex-wrap items-center gap-2 mb-6">
        <div className="w-48">
          <Select
            value={modelFilter}
            onValueChange={(v) => updateFilter('model', v)}
            options={modelOptions}
            placeholder="All models"
          />
        </div>
        <div className="w-36">
          <Input
            placeholder="Tag"
            value={tagFilter}
            onChange={(e) => updateFilter('tag', e.target.value)}
          />
        </div>
        <div className="w-40">
          <Input
            placeholder="Virtual key"
            value={virtualKeyFilter}
            onChange={(e) => updateFilter('virtual_key', e.target.value)}
          />
        </div>
        <div className="w-36">
          <Select
            value={streamedFilter}
            onValueChange={(v) => updateFilter('streamed', v)}
            options={streamedOptions}
            placeholder="Streamed"
          />
        </div>
        {hasActiveFilters && (
          <Button size="sm" variant="ghost" onClick={clearFilters}>
            Clear filters
          </Button>
        )}
      </div>

      {/* Table Panel */}
      <Panel noPadding>
        {loading ? (
          <TableSkeleton rows={8} />
        ) : error ? (
          <ErrorState
            title="Couldn't load captures"
            message={error}
            onRetry={fetchCaptures}
          />
        ) : captures.length === 0 && (hasActiveFilters || page > 0) ? (
          <EmptyState
            title="No captures match"
            description="Nothing recorded matches these filters."
            actionLabel="Clear filters"
            onAction={clearFilters}
          />
        ) : captures.length === 0 ? (
          <div className="p-6 text-left">
            <div className="text-[14px] font-medium text-text">No captures yet</div>
            <div className="text-[13px] text-text-2 mt-1 max-w-[560px]">
              Captures appear when chat requests pass through the gateway. Send{' '}
              {SAMPLE_TRAFFIC_COUNT} sample requests to a real model to see it work, or point
              your own client at the gateway.
            </div>
            <div className="mt-4 flex items-center gap-3">
              <Button
                size="sm"
                variant="primary"
                disabled={sampleSent !== null || models.length === 0}
                onClick={handleSendSample}
              >
                Send sample traffic
              </Button>
              <Button size="sm" variant="secondary" disabled={sampleSent !== null} onClick={fetchCaptures}>
                Refresh
              </Button>
              {sampleSent !== null && (
                <span className="text-[12px] text-text-2 tabular-nums" role="status">
                  {sampleSent === 0
                    ? 'Finding a model that answers…'
                    : `Sent ${sampleSent} of ${SAMPLE_TRAFFIC_COUNT}…`}
                </span>
              )}
            </div>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={captures}
            selectedId={selectedCapture?.id}
            getRowId={(c) => c.id}
            onRowClick={openDrawer}
            pageIndex={page}
            pageSize={50}
            totalCount={captures.length >= 50 ? (page + 2) * 50 : (page * 50) + captures.length}
            onPageChange={(p) => {
              const next = new URLSearchParams(searchParams);
              next.set('page', String(p));
              setSearchParams(next);
            }}
          />
        )}
      </Panel>

      {/* Drawer */}
      <CaptureDrawer
        capture={selectedCapture}
        isOpen={Boolean(paramId && selectedCapture)}
        onClose={closeDrawer}
        onNavigatePrev={currentIndex > 0 ? navigatePrev : undefined}
        onNavigateNext={
          currentIndex >= 0 && currentIndex < captures.length - 1
            ? navigateNext
            : undefined
        }
      />
    </div>
  );
};

