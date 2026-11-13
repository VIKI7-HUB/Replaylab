import React from 'react';

export interface PageHeaderProps {
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  statusLabel?: React.ReactNode;
  className?: string;
}

export const PageHeader: React.FC<PageHeaderProps> = ({
  title,
  description,
  actions,
  statusLabel,
  className = '',
}) => {
  return (
    // On narrow screens the actions wrap below the title instead of running off the page.
    <div className={`mb-6 flex flex-wrap items-start justify-between gap-x-4 gap-y-3 ${className}`}>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="page-title text-text m-0">{title}</h1>
          {statusLabel}
        </div>
        {description && (
          <p className="text-[13px] text-text-2 mt-1 m-0">{description}</p>
        )}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
};

