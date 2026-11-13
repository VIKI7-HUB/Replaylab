import React, { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { HamburgerMenuIcon, MagnifyingGlassIcon, SunIcon, MoonIcon } from '@radix-ui/react-icons';
import { useTheme } from './theme';
import { Tooltip } from '../components/Tooltip';
import { ServiceHealth, insightsHealth, replayLabHealth, staleHealth } from '../api/health';

export interface TopBarProps {
  onOpenSearch: () => void;
  /** Opens the sidebar as a slide-out menu (small screens only). */
  onOpenMenu?: () => void;
}

const CHECKING: ServiceHealth = { status: 'degraded', text: 'Checking…' };

export const TopBar: React.FC<TopBarProps> = ({ onOpenSearch, onOpenMenu }) => {
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
  let current = 'Overview';

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
    <header className="h-12 bg-bg border-b border-border px-4 md:px-6 flex items-center justify-between gap-3 shrink-0">
      {/* Menu button (small screens) and breadcrumb */}
      <div className="flex items-center gap-1.5 text-[13px] min-w-0">
        <button
          type="button"
          onClick={onOpenMenu}
          aria-label="Open menu"
          className="md:hidden -ml-1 mr-1 w-8 h-8 inline-flex items-center justify-center rounded text-text-2 hover:bg-bg-muted hover:text-text transition-colors shrink-0"
        >
          <HamburgerMenuIcon className="w-[16px] h-[16px]" />
        </button>
        {parent ? (
          <>
            <span className="text-text-2 hidden sm:inline">{parent}</span>
            <span className="text-text-3 hidden sm:inline">/</span>
            <span className="text-text font-medium truncate">{current}</span>
          </>
        ) : (
          <span className="text-text font-medium truncate">{current}</span>
        )}
      </div>

      {/* Right controls */}
      <div className="flex items-center gap-3 md:gap-4 shrink-0">
        {/* Search button */}
        <button
          type="button"
          onClick={onOpenSearch}
          aria-label="Search"
          className="h-7 w-8 md:w-60 px-2 md:px-2.5 rounded border border-border-strong bg-bg flex items-center justify-between text-[13px] text-text-3 hover:bg-bg-muted transition-colors cursor-pointer"
        >
          <div className="flex items-center gap-2">
            <MagnifyingGlassIcon className="w-[15px] h-[15px] text-text-3 shrink-0" />
            <span className="hidden md:inline">Search…</span>
          </div>
          <kbd className="hidden md:inline mono-ui text-[12px] bg-bg-muted px-1 rounded-sm border border-border">
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

