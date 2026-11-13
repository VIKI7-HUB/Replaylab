import React from 'react';
import { Button } from './Button';

export interface EmptyStateProps {
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  title,
  description,
  actionLabel,
  onAction,
  className = '',
}) => {
  return (
    <div className={`p-6 text-left ${className}`}>
      <div className="text-[14px] font-medium text-text">{title}</div>
      <div className="text-[13px] text-text-2 mt-1">{description}</div>
      {actionLabel && onAction && (
        <div className="mt-4">
          <Button size="sm" variant="secondary" onClick={onAction}>
            {actionLabel}
          </Button>
        </div>
      )}
    </div>
  );
};

export interface ErrorStateProps {
  title?: string;
  message: string;
  onRetry?: () => void;
  className?: string;
}

export const ErrorState: React.FC<ErrorStateProps> = ({
  title = "Couldn't load data",
  message,
  onRetry,
  className = '',
}) => {
  return (
    <div className={`p-6 text-left ${className}`}>
      <div className="text-[14px] font-medium text-danger">{title}</div>
      <div className="mono-ui text-text-2 mt-1">{message}</div>
      {onRetry && (
        <div className="mt-4">
          <Button size="sm" variant="secondary" onClick={onRetry}>
            Retry
          </Button>
        </div>
      )}
    </div>
  );
};

export interface OfflineStateProps {
  insightsUrl?: string;
  onRetry?: () => void;
  extraActions?: React.ReactNode;
  className?: string;
}

export const AnalyticsOfflineState: React.FC<OfflineStateProps> = ({
  insightsUrl = import.meta.env.VITE_INSIGHTS_URL || 'http://localhost:8090',
  onRetry,
  extraActions,
  className = '',
}) => {
  return (
    <div className={`p-6 text-left ${className}`}>
      <div className="text-[14px] font-medium text-text">Analytics offline</div>
      <div className="text-[13px] text-text-2 mt-1">
        The insights service at {insightsUrl} isn't reachable. Live features still work.
      </div>
      <div className="flex items-center gap-2 mt-4">
        {onRetry && (
          <Button size="sm" variant="secondary" onClick={onRetry}>
            Retry
          </Button>
        )}
        {extraActions}
      </div>
    </div>
  );
};

export const Skeleton: React.FC<{
  className?: string;
  height?: string;
  width?: string;
}> = ({ className = '', height = 'h-4', width = 'w-full' }) => {
  return (
    <div
      className={`bg-bg-muted rounded-sm animate-skeleton ${height} ${width} ${className}`}
    />
  );
};

export const TableSkeleton: React.FC<{ rows?: number }> = ({ rows = 5 }) => {
  return (
    <div className="w-full divide-y divide-border">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="h-9 px-3 flex items-center gap-4">
          <Skeleton width="w-20" height="h-4" />
          <Skeleton width="w-32" height="h-4" />
          <Skeleton width="w-16" height="h-4" />
          <Skeleton width="w-24" height="h-4" />
          <Skeleton width="w-16" height="h-4" />
        </div>
      ))}
    </div>
  );
};

