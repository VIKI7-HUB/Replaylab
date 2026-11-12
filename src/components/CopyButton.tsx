import React, { useState } from 'react';
import { CopyIcon, CheckIcon } from '@radix-ui/react-icons';
import { Tooltip } from './Tooltip';

export interface CopyButtonProps {
  value: string;
  tooltipText?: string;
  className?: string;
}

export const CopyButton: React.FC<CopyButtonProps> = ({
  value,
  tooltipText = 'Copy',
  className = '',
}) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <Tooltip content={copied ? 'Copied' : tooltipText}>
      <button
        type="button"
        onClick={handleCopy}
        aria-label={tooltipText}
        className={`inline-flex items-center justify-center w-7 h-7 rounded text-text-2 hover:bg-bg-muted hover:text-text transition-colors ${className}`}
      >
        {copied ? (
          <CheckIcon className="w-[15px] h-[15px] text-success" />
        ) : (
          <CopyIcon className="w-[15px] h-[15px]" />
        )}
      </button>
    </Tooltip>
  );
};

