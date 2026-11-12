import React from 'react';
import {
  CheckCircledIcon,
  ExclamationTriangleIcon,
  CrossCircledIcon,
  MinusCircledIcon,
} from '@radix-ui/react-icons';

export type StatusValue =
  | 'SAFE_TO_SWITCH'
  | 'SWITCH_WITH_CAUTION'
  | 'DO_NOT_SWITCH'
  | 'running'
  | 'completed'
  | 'cancelled'
  | 'budget_exhausted'
  | 'failed'
  | 'CURRENT'
  | 'STALE'
  | 'PARTIALLY_STALE'
  | 'UNVERIFIABLE'
  | 'skipped'
  | 'not_checked'
  | 'complete'
  | 'error'
  | 'abandoned'
  | 'truncated'
  | 'llm'
  | 'heuristic'
  | string
  | null
  | undefined;

export interface StatusLabelProps {
  value: StatusValue;
  stopReason?: string | null;
  customText?: string;
  className?: string;
}

export const StatusLabel: React.FC<StatusLabelProps> = ({
  value,
  stopReason,
  customText,
  className = '',
}) => {
  let text = customText || '';
  let tone: 'success' | 'warning' | 'danger' | 'neutral' | 'running' = 'neutral';
  let icon: React.ReactNode = null;

  const normalized = value ? String(value) : 'skipped';

  switch (normalized) {
    case 'SAFE_TO_SWITCH':
      text = text || 'Safe to switch';
      tone = 'success';
      icon = <CheckCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'SWITCH_WITH_CAUTION':
      text = text || 'Switch with caution';
      tone = 'warning';
      icon = <ExclamationTriangleIcon className="w-[15px] h-[15px]" />;
      break;
    case 'DO_NOT_SWITCH':
      text = text || 'Do not switch';
      tone = 'danger';
      icon = <CrossCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'running':
      text = text || 'Running';
      tone = 'running';
      icon = <span className="w-1.5 h-1.5 rounded-full bg-accent animate-running-dot" />;
      break;
    case 'completed':
      text = text || 'Completed';
      tone = 'success';
      icon = <CheckCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'cancelled':
      text = text || 'Cancelled';
      tone = 'neutral';
      icon = <MinusCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'budget_exhausted':
      text = text || 'Budget reached';
      tone = 'warning';
      icon = <ExclamationTriangleIcon className="w-[15px] h-[15px]" />;
      break;
    case 'failed':
      text = text || (stopReason?.startsWith('interrupted:') ? 'Interrupted' : 'Failed');
      tone = 'danger';
      icon = <CrossCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'CURRENT':
      text = text || 'Current';
      tone = 'success';
      icon = <CheckCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'STALE':
      text = text || 'Stale';
      tone = 'danger';
      icon = <CrossCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'PARTIALLY_STALE':
      text = text || 'Partly stale';
      tone = 'warning';
      icon = <ExclamationTriangleIcon className="w-[15px] h-[15px]" />;
      break;
    case 'UNVERIFIABLE':
      text = text || 'Unverifiable';
      tone = 'neutral';
      icon = <MinusCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'skipped':
    case 'not_checked':
    case 'null':
      text = text || 'Not checked';
      tone = 'neutral';
      icon = (
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" className="shrink-0">
          <circle cx="6" cy="6" r="5" stroke="var(--text-3)" strokeWidth="1.5" strokeDasharray="2 2" />
        </svg>
      );
      break;
    case 'complete':
      text = text || 'Complete';
      tone = 'neutral';
      icon = <CheckCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'error':
      text = text || 'Error';
      tone = 'danger';
      icon = <CrossCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'abandoned':
      text = text || 'Abandoned';
      tone = 'warning';
      icon = <ExclamationTriangleIcon className="w-[15px] h-[15px]" />;
      break;
    case 'truncated':
      text = text || 'Truncated';
      tone = 'warning';
      icon = <ExclamationTriangleIcon className="w-[15px] h-[15px]" />;
      break;
    case 'llm':
      text = text || 'LLM verified';
      tone = 'neutral';
      icon = <CheckCircledIcon className="w-[15px] h-[15px]" />;
      break;
    case 'heuristic':
      text = text || 'Heuristic only';
      tone = 'warning';
      icon = <ExclamationTriangleIcon className="w-[15px] h-[15px]" />;
      break;
    default:
      text = text || normalized;
      tone = 'neutral';
      icon = <MinusCircledIcon className="w-[15px] h-[15px]" />;
      break;
  }

  const toneClasses = {
    success: 'bg-success-bg text-success',
    warning: 'bg-warning-bg text-warning',
    danger: 'bg-danger-bg text-danger',
    neutral: 'bg-bg-muted text-text-2',
    running: 'bg-accent-bg text-accent',
  }[tone];

  return (
    <span
      className={`inline-flex items-center gap-1.5 h-5 px-1.5 rounded-sm text-[12px] font-medium shrink-0 whitespace-nowrap ${toneClasses} ${className}`}
    >
      {icon}
      <span>{text}</span>
    </span>
  );
};

