import React, { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { MagnifyingGlassIcon, SunIcon, MoonIcon } from '@radix-ui/react-icons';
import { useTheme } from './theme';
import { Tooltip } from '../components/Tooltip';
import { ServiceHealth, insightsHealth, replayLabHealth, staleHealth } from '../api/health';
import { CONSOLE_NAME } from './consoles';

export interface TopBarProps {
  onOpenSearch: () => void;
}

const CHECKING: ServiceHealth = { status: 'degraded', text: 'Checking…' };

export const TopBar: React.FC<TopBarProps> = ({ onOpenSearch }) => {
  const location = useLocation();
  const { theme, toggleTheme } = useTheme();

  const [health, setHealth] = useState<ServiceHealth[]>([CHECKING, CHECKING, CHECKING]);

  const pollHealth = async () => {
    setHealth(await Promise.all([replayLabHealth(), staleHealth(), insightsHealth()]));
  };

  useEffect(() => {
    pollHealth();
    const interval = setInterval(pollHealth, 15000);
    return () => clearInterval(interval);
  }, []);

  // Compute breadcrumb
  const path = location.pathname;
  let parent = '';
  let current = CONSOLE_NAME;

  if (path.startsWith('/replaylab/captures')) {
    parent = 'Replay Lab';
    current = 'Captures';
  } else if (path.startsWith('/replaylab/experiments/new')) {
    parent = 'Experiments';
    current = 'New experiment';
  } else if (path.startsWith('/replaylab/experiments/')) {
    parent = 'Experiments';
    current = 'Detail';
  } else if (path.startsWith('/replaylab/experiments')) {
    parent = 'Replay Lab';
    current = 'Experiments';
  } else if (path.startsWith('/replaylab/settings')) {
    parent = 'Replay Lab';
    current = 'Settings';
  } else if (path.startsWith('/stale/check')) {
    parent = 'Stale answers';
    current = 'Check an answer';
  } else if (path.startsWith('/stale/reports')) {
    parent = 'Stale answers';
    current = 'Report';
  } else if (path.startsWith('/stale/history')) {
    parent = 'Stale answers';
    current = 'History';
  }

  const getDotColor = (status: 'healthy' | 'degraded' | 'offline') => {
    if (status === 'healthy') return 'bg-success';
    if (status === 'degraded') return 'bg-warning';
    return 'bg-danger';
  };

  return (
    <header className="h-12 bg-bg border-b border-border px-6 flex items-center justify-between shrink-0">
      {/* Breadcrumb */}
      <div className="flex items-center gap-1.5 text-[13px]">
        {parent ? (
          <>
            <span className="text-text-2">{parent}</span>
            <span className="text-text-3">/</span>
            <span className="text-text font-medium">{current}</span>
          </>
        ) : (
          <span className="text-text font-medium">{current}</span>
        )}
      </div>

      {/* Right controls */}
      <div className="flex items-center gap-4">
        {/* Search button */}
        <button
          type="button"
          onClick={onOpenSearch}
          className="h-7 w-60 px-2.5 rounded border border-border-strong bg-bg flex items-center justify-between text-[13px] text-text-3 hover:bg-bg-muted transition-colors cursor-pointer"
        >
          <div className="flex items-center gap-2">
            <MagnifyingGlassIcon className="w-[15px] h-[15px] text-text-3 shrink-0" />
            <span>Search…</span>
          </div>
          <kbd className="mono-ui text-[12px] bg-bg-muted px-1 rounded-sm border border-border">
            {/Mac|iPhone|iPad/.test(navigator.platform) ? '⌘K' : 'Ctrl K'}
          </kbd>
        </button>

        {/* Health dots 8px apart, one per configured service */}
        <div className="flex items-center gap-2">
          {health.filter((h) => h.configured !== false).map((h, i) => (
            <Tooltip key={i} content={h.text}>
              <span className={`w-2 h-2 rounded-full cursor-help ${getDotColor(h.status)}`} />
            </Tooltip>
          ))}
        </div>

        {/* Theme toggle */}
        <Tooltip content={theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'}>
          <button
            type="button"
            onClick={toggleTheme}
            aria-label="Toggle theme"
            className="w-7 h-7 inline-flex items-center justify-center rounded text-text-2 hover:bg-bg-muted hover:text-text transition-colors"
          >
            {theme === 'light' ? (
              <MoonIcon className="w-[15px] h-[15px]" />
            ) : (
              <SunIcon className="w-[15px] h-[15px]" />
            )}
          </button>
        </Tooltip>
      </div>
    </header>
  );
};

