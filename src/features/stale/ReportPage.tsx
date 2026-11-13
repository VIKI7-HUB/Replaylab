import React, { useState, useEffect } from 'react';
import { useParams, useLocation } from 'react-router-dom';
import {
  ChevronRightIcon,
  ExternalLinkIcon,
  CheckIcon,
  CopyIcon,
} from '@radix-ui/react-icons';
import { PageHeader } from '../../app/PageHeader';
import { Button } from '../../components/Button';
import { Panel } from '../../components/Panel';
import { StatStrip } from '../../components/StatStrip';
import { StatusLabel } from '../../components/StatusLabel';
import { FreshnessMeter } from '../../components/Meter';
import { Tabs } from '../../components/Tabs';
import { Tooltip } from '../../components/Tooltip';
import { ErrorState, Skeleton } from '../../components/States';
import { CheckReport, getReportById } from '../../api/stale';
import { formatIsoUtc, formatRelativeTime } from '../../lib/format';
import { useToast } from '../../components/Toast';

export const ReportPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const location = useLocation();
  const { toast } = useToast();

  const [report, setReport] = useState<CheckReport | null>(() => {
    // Check if report was passed via router state from CheckPage (§14.1)
    const state = location.state as { report?: CheckReport } | null;
    return state?.report ?? null;
  });

  const [loading, setLoading] = useState(!report);
  const [error, setError] = useState<string | null>(null);
  const [copiedAnswer, setCopiedAnswer] = useState(false);

  // Expanded claim accordion state
  const [expandedClaims, setExpandedClaims] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (report || !id) return;

    setLoading(true);
    getReportById(id)
      .then((rep) => setReport(rep))
      .catch((err) => setError(err.message || "Couldn't load report"))
      .finally(() => setLoading(false));
  }, [id, report]);

  const handleCopyCorrected = async () => {
    if (!report?.corrected_answer) return;
    try {
      await navigator.clipboard.writeText(report.corrected_answer);
      setCopiedAnswer(true);
      toast('Corrected answer copied to clipboard', 'success');
      setTimeout(() => setCopiedAnswer(false), 1500);
    } catch {
      toast('Failed to copy', 'danger');
    }
  };

  const scrollToClaim = (claimId: string) => {
    setExpandedClaims((prev) => ({ ...prev, [claimId]: true }));
    const el = document.getElementById(`claim-${claimId}`);
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton height="h-8" width="w-48" />
        <Skeleton height="h-4" width="w-96" />
        <Skeleton height="h-28" />
        <Skeleton height="h-64" />
      </div>
    );
  }

  if (error || !report) {
    return (
      <ErrorState
        title="Couldn't load report"
        message={error || 'Report not found'}
        onRetry={() => {
          if (id) {
            setLoading(true);
            setError(null);
            getReportById(id)
              .then(setReport)
              .catch((e) => setError(e.message))
              .finally(() => setLoading(false));
          }
        }}
      />
    );
  }

  // Slice and highlight original answer text using quoted_span
  // Sort claims by start offset ascending
  // Claims the service couldn't locate in the answer have no span and aren't highlighted.
  const sortedClaims = report.claims
    .flatMap((claim) => (claim.quoted_span ? [{ ...claim, quoted_span: claim.quoted_span }] : []))
    .sort((a, b) => a.quoted_span.start - b.quoted_span.start);

  const renderHighlightedAnswer = () => {
    const raw = report.original_answer || '';
    if (!sortedClaims.length) {
      return <div className="text-[15px] leading-[26px] text-text">{raw}</div>;
    }

    const segments: React.ReactNode[] = [];
    let curIndex = 0;

    sortedClaims.forEach((claim, idx) => {
      const { start, end } = claim.quoted_span;
      // Add text before this claim
      if (start > curIndex) {
        segments.push(raw.slice(curIndex, start));
      }

      const claimText = raw.slice(start, end);
      const isSkipped = claim.skipped;
      const verdict = claim.verdict;

      let underlineClass = 'underline decoration-2 underline-offset-[3px]';
      if (isSkipped || !verdict) {
        underlineClass += ' decoration-[var(--text-3)] decoration-dashed';
      } else if (verdict === 'CURRENT') {
        underlineClass += ' decoration-success';
      } else if (verdict === 'STALE') {
        underlineClass += ' decoration-danger';
      } else if (verdict === 'PARTIALLY_STALE') {
        underlineClass += ' decoration-warning';
      } else {
        underlineClass += ' decoration-text-3';
      }

      segments.push(
        <Tooltip
          key={`${claim.id}-${idx}`}
          content={
            <div className="flex items-center gap-2">
              <StatusLabel value={claim.skipped ? 'skipped' : claim.verdict} />
              <span className="tabular-nums font-medium text-[12px]">
                {claim.freshness_score.toFixed(1)}
              </span>
            </div>
          }
        >
          <mark
            onClick={() => scrollToClaim(claim.id)}
            className={`bg-transparent text-text cursor-pointer hover:bg-bg-muted ${underlineClass}`}
          >
            {claimText}
          </mark>
        </Tooltip>
      );

      curIndex = Math.max(curIndex, end);
    });

    if (curIndex < raw.length) {
      segments.push(raw.slice(curIndex));
    }

    return (
      <div className="text-[15px] leading-[26px] text-text whitespace-pre-wrap">
        {segments}
      </div>
    );
  };

  // The corrected answer is light markdown: prose with [n] citations, then a
  // "### Verified Sources" list of "- [n] [title](url)" lines.
  const renderInline = (text: string, keyPrefix: string) =>
    text.split(/(\[\d+\]|\[[^\]]+\]\(https?:\/\/[^\s)]+\)|\*\*[^*]+\*\*)/g).map((part, i) => {
      const key = `${keyPrefix}-${i}`;
      const cite = part.match(/^\[(\d+)\]$/);
      if (cite) {
        return (
          <sup key={key} className="text-[11px] font-medium text-accent px-0.5">
            [{cite[1]}]
          </sup>
        );
      }
      const link = part.match(/^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)$/);
      if (link) {
        return (
          <a
            key={key}
            href={link[2]}
            target="_blank"
            rel="noopener noreferrer"
            className="text-accent hover:underline"
          >
            {link[1]}
          </a>
        );
      }
      const bold = part.match(/^\*\*([^*]+)\*\*$/);
      if (bold) return <strong key={key}>{bold[1]}</strong>;
      return part;
    });

  const renderCorrectedAnswer = () => {
    const lines = (report.corrected_answer || '').split('\n');
    return (
      <div className="text-[14px] leading-6 text-text">
        {lines.map((line, i) => {
          const heading = line.match(/^#{1,6}\s+(.*)$/);
          if (heading) {
            return (
              <div key={i} className="text-[12px] font-medium text-text-2 mt-4 mb-1">
                {heading[1]}
              </div>
            );
          }
          const bullet = line.match(/^\s*[-*]\s+(.*)$/);
          if (bullet) {
            return (
              <div key={i} className="text-[13px] text-text-2 break-words">
                {renderInline(bullet[1], `l${i}`)}
              </div>
            );
          }
          if (!line.trim()) return <div key={i} className="h-3" />;
          return (
            <p key={i} className="m-0 whitespace-pre-wrap">
              {renderInline(line, `l${i}`)}
            </p>
          );
        })}
      </div>
    );
  };

  const totalClaims = report.summary.total_claims;
  const skippedClaims = report.summary.skipped;
  const activeClaims = totalClaims - skippedClaims;
  const staleCount = report.summary.stale + report.summary.partially_stale;

  return (
    <div className="space-y-6">
      {/* Header */}
      <PageHeader
        title="Report"
        statusLabel={<StatusLabel value={report.verification_mode} />}
        description={
          <span className="mono-ui text-[12.5px] text-text-2">
            {report.report_id} · checked {formatIsoUtc(report.checked_at)} (
            {formatRelativeTime(report.checked_at)})
          </span>
        }
        actions={
          <Button size="md" variant="secondary" onClick={handleCopyCorrected}>
            {copiedAnswer ? (
              <CheckIcon className="w-[15px] h-[15px] text-success mr-1.5" />
            ) : (
              <CopyIcon className="w-[15px] h-[15px] mr-1.5" />
            )}
            Copy corrected answer
          </Button>
        }
      />

      {/* Stat Strip */}
      <StatStrip
        cells={[
          {
            label: 'Freshness',
            value: (
              <div className="flex items-center gap-2">
                <span>{report.overall_freshness_score.toFixed(1)}</span>
                <FreshnessMeter score={report.overall_freshness_score} showNumber={false} />
              </div>
            ),
            detail: '0-100, higher is fresher',
          },
          {
            label: 'Claims',
            value: totalClaims,
            detail: `${activeClaims} checked · ${skippedClaims} skipped`,
          },
          {
            label: 'Stale',
            value: (
              <span className={staleCount > 0 ? 'text-danger' : 'text-text'}>
                {staleCount}
              </span>
            ),
            detail: `${report.summary.stale} stale, ${report.summary.partially_stale} partly`,
          },
          {
            label: 'Sources used',
            value: report.usage.tavily_calls,
            detail: `${report.usage.tavily_calls_saved} served from cache`,
          },
        ]}
      />

      {/* Answer Panel ("Original answer") */}
      <Panel title="Original answer">
        {renderHighlightedAnswer()}
      </Panel>

      {/* Claims Panel */}
      <Panel title="Claims" noPadding>
        <div className="divide-y divide-border">
          {report.claims.map((claim) => {
            const isOpen = expandedClaims[claim.id] || false;
            const toggle = () =>
              setExpandedClaims((prev) => ({ ...prev, [claim.id]: !isOpen }));

            return (
              <div id={`claim-${claim.id}`} key={claim.id} className="transition-colors">
                {/* Collapsible header row */}
                <div
                  onClick={toggle}
                  className="p-4 flex items-center justify-between cursor-pointer hover:bg-bg-muted"
                >
                  <div className="flex items-center gap-3 truncate pr-4">
                    <ChevronRightIcon
                      className={`w-4 h-4 text-text-2 shrink-0 transition-transform ${
                        isOpen ? 'rotate-90' : ''
                      }`}
                    />
                    <span className="text-[13px] text-text truncate">{claim.text}</span>
                    <span className="text-[12px] text-text-3 shrink-0">
                      {claim.claim_type}
                    </span>
                  </div>

                  <div className="flex items-center gap-4 shrink-0">
                    <FreshnessMeter score={claim.freshness_score} />
                    <StatusLabel
                      value={claim.skipped ? 'skipped' : claim.verdict}
                    />
                  </div>
                </div>

                {/* Expanded Details (§14.2) */}
                {isOpen && (
                  <div className="pl-8 pr-4 pb-4 space-y-4 bg-bg-subtle border-t border-border pt-3 animate-fade-in">
                    {claim.skipped ? (
                      <div className="text-[13px] text-text-2">
                        Not checked: {claim.skip_reason || 'Low staleness risk.'}
                      </div>
                    ) : (
                      <>
                        {/* Current value */}
                        {claim.current_value && (
                          <div>
                            <div className="text-[12px] font-medium text-text-2 mb-1">
                              Current value
                            </div>
                            <div className="text-[14px] text-text font-medium">
                              {claim.current_value}
                            </div>
                          </div>
                        )}

                        {/* Explanation (italic) */}
                        {claim.explanation && (
                          <div>
                            <div className="text-[12px] font-medium text-text-2 mb-1">
                              Explanation
                            </div>
                            <blockquote className="m-0 text-[13px] italic text-text-2 pl-3 border-l-2 border-border-strong">
                              "{claim.explanation}"
                            </blockquote>
                          </div>
                        )}

                        {/* Score breakdown: 2-column table + formula */}
                        {claim.score_breakdown && (
                          <div>
                            <div className="text-[12px] font-medium text-text-2 mb-2">
                              Score breakdown
                            </div>
                            <div className="grid grid-cols-2 gap-2 bg-bg p-3 rounded-md border border-border text-[12px] max-w-md">
                              <span className="text-text-2">Evidence agreement</span>
                              <span className="text-text text-right tabular-nums">
                                {claim.score_breakdown.evidence_agreement}
                              </span>

                              <span className="text-text-2">Source authority</span>
                              <span className="text-text text-right tabular-nums">
                                {claim.score_breakdown.source_authority}
                              </span>

                              <span className="text-text-2">Recency</span>
                              <span className="text-text text-right tabular-nums">
                                {claim.score_breakdown.recency_factor}
                              </span>

                              <span className="text-text-2">Search relevance</span>
                              <span className="text-text text-right tabular-nums">
                                {claim.score_breakdown.tavily_relevance}
                              </span>

                              <span className="text-text-2">Verdict factor</span>
                              <span className="text-text text-right tabular-nums">
                                {claim.score_breakdown.verdict_score}
                              </span>
                            </div>
                            {claim.score_breakdown.formula && (
                              <div className="mono-ui text-[12px] text-text-3 mt-2">
                                Formula: {claim.score_breakdown.formula}
                              </div>
                            )}
                          </div>
                        )}

                        {/* Evidence list */}
                        {claim.evidence && claim.evidence.length > 0 && (
                          <div>
                            <div className="text-[12px] font-medium text-text-2 mb-2">
                              Evidence
                            </div>
                            <div className="space-y-3">
                              {claim.evidence.map((ev, eIdx) => (
                                <div
                                  key={eIdx}
                                  className="bg-bg p-3 rounded-md border border-border space-y-2"
                                >
                                  <div className="flex flex-wrap items-center gap-2 justify-between">
                                    <div className="flex items-center gap-1.5 truncate">
                                      <a
                                        href={ev.url}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        className="text-[13px] text-accent hover:underline inline-flex items-center gap-1 truncate"
                                      >
                                        <span className="truncate">{ev.title || ev.url}</span>
                                        <ExternalLinkIcon className="w-3 h-3 shrink-0" />
                                      </a>
                                      <span className="text-[12px] text-text-3">
                                        ({ev.domain})
                                      </span>
                                    </div>
                                    {ev.is_official && (
                                      <StatusLabel
                                        value="complete"
                                        customText="Official source"
                                      />
                                    )}
                                  </div>

                                  {ev.quote && (
                                    <div className="text-[13px] text-text-2 pl-3 border-l-2 border-border-strong">
                                      {ev.quote}
                                    </div>
                                  )}
                                </div>
                              ))}
                            </div>
                          </div>
                        )}
                      </>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </Panel>

      {/* Corrected answer panel: Tabs Corrected | Changes */}
      <Panel title="Corrected answer">
        <Tabs
          tabs={[
            {
              value: 'corrected',
              label: 'Corrected',
              content: renderCorrectedAnswer(),
            },
            {
              value: 'changes',
              label: 'Changes',
              content: (
                <div className="space-y-3">
                  {report.changes.length === 0 ? (
                    <div className="text-[13px] text-text-3">No changes made.</div>
                  ) : (
                    report.changes.map((ch, idx) => (
                      <div
                        key={idx}
                        className="bg-bg p-3 rounded-md border border-border text-[13px] space-y-2"
                      >
                        <div className="flex flex-col gap-1.5">
                          <div className="p-1.5 rounded bg-danger-bg text-danger line-through">
                            {ch.original_claim}
                          </div>
                          <div className="p-1.5 rounded bg-success-bg text-success">
                            {ch.updated_claim}
                          </div>
                        </div>
                        <div className="flex items-center justify-between text-[12px] text-text-3">
                          <span>{ch.reason}</span>
                          <span className="text-accent font-medium">
                            Source [{ch.citation_index}]
                          </span>
                        </div>
                      </div>
                    ))
                  )}
                </div>
              ),
            },
          ]}
        />
      </Panel>
    </div>
  );
};

