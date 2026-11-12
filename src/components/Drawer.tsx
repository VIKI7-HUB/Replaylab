import React, { useEffect } from 'react';
import { Cross2Icon } from '@radix-ui/react-icons';
import { CopyButton } from './CopyButton';
import { Tooltip } from './Tooltip';

export interface DrawerProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
  children: React.ReactNode;
}

export const Drawer: React.FC<DrawerProps> = ({
  isOpen,
  onClose,
  title,
  onNavigatePrev,
  onNavigateNext,
  children,
}) => {
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't trigger if user is typing in an input or textarea
      const target = e.target as HTMLElement;
      if (['INPUT', 'TEXTAREA'].includes(target.tagName) || target.isContentEditable) {
        return;
      }

      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      } else if (e.key === 'ArrowUp' && onNavigatePrev) {
        e.preventDefault();
        onNavigatePrev();
      } else if (e.key === 'ArrowDown' && onNavigateNext) {
        e.preventDefault();
        onNavigateNext();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, onNavigatePrev, onNavigateNext]);

  if (!isOpen) return null;

  return (
    <aside
      className="fixed top-12 right-0 bottom-0 z-30 w-full md:w-[640px] bg-bg border-l border-border flex flex-col shadow-none animate-drawer"
      aria-label="Detail drawer"
    >
      {/* Header 52px */}
      <div className="h-[52px] px-5 border-b border-border flex items-center justify-between shrink-0">
        <div className="flex items-center gap-2 overflow-hidden">
          <span className="section-title text-text truncate mono-ui">{title}</span>
          <CopyButton value={title} tooltipText="Copy ID" />
        </div>
        <Tooltip content="Close drawer">
          <button
            type="button"
            onClick={onClose}
            aria-label="Close drawer"
            className="w-7 h-7 inline-flex items-center justify-center rounded text-text-2 hover:bg-bg-muted hover:text-text transition-colors"
          >
            <Cross2Icon className="w-[15px] h-[15px]" />
          </button>
        </Tooltip>
      </div>

      {/* Body with scrolling and sections 24px apart */}
      <div className="flex-1 overflow-y-auto p-5 space-y-6">{children}</div>
    </aside>
  );
};

