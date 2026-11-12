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
    <div className={`mb-6 flex items-start justify-between ${className}`}>
      <div>
        <div className="flex items-center gap-2">
          <h1 className="page-title text-text m-0">{title}</h1>
          {statusLabel}
        </div>
        {description && (
          <p className="text-[13px] text-text-2 mt-1 m-0">{description}</p>
        )}
      </div>
      {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
    </div>
  );
};

