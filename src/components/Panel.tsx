import React from 'react';

export interface PanelProps {
  title?: React.ReactNode;
  actions?: React.ReactNode;
  children: React.ReactNode;
  noPadding?: boolean;
  className?: string;
}

export const Panel: React.FC<PanelProps> = ({
  title,
  actions,
  children,
  noPadding = false,
  className = '',
}) => {
  return (
    <div className={`bg-bg border border-border rounded-md ${className}`}>
      {(title || actions) && (
        <div className="h-11 px-4 border-b border-border flex items-center justify-between">
          <div className="section-title text-text flex items-center gap-2">{title}</div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>
      )}
      <div className={noPadding ? '' : 'p-4'}>{children}</div>
    </div>
  );
};

