import React, { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { MagnifyingGlassIcon, SunIcon, MoonIcon } from '@radix-ui/react-icons';
import { useTheme } from './theme';
import { Tooltip } from '../components/Tooltip';
import { getHealth as getReplayLabHealth } from '../api/replaylab';
import { getStaleHealth } from '../api/stale';
import { getInsightsHealth } from '../api/insights';

export interface TopBarProps {
  onOpenSearch: () => void;
}

export const TopBar: React.FC<TopBarProps> = ({ onOpenSearch }) => {
  const location = useLocation();
  const { theme, toggleTheme } = useTheme();

  // Health states
  const [replayLabHealth, setReplayLabHealth] = useState<{ status: 'healthy' | 'degraded' | 'offline'; text: string }>({
    status: 'healthy',
    text: 'Replay Lab: connecting…',
  });
  const [staleHealth, setStaleHealth] = useState<{ status: 'healthy' | 'degraded' | 'offline'; text: string }>({
    status: 'healthy',
    text: 'Stale-Answer Detector: connecting…',
  });
  const [insightsHealth, setInsightsHealth] = useState<{ status: 'healthy' | 'offline'; text: string }>({
    status: 'offline',
    text: 'Insights: checking…',
  });

  const pollHealth = async () => {
    const [replayRes, staleRes, insightsRes] = await Promise.allSettled([
      getReplayLabHealth(),
      getStaleHealth(),
      getInsightsHealth(),
    ]);

    // 1. Replay Lab
    if (replayRes.status === 'fulfilled') {
      const h = replayRes.value;
      setReplayLabHealth({
        status: 'healthy',
        text: `Replay Lab: healthy · ${h.stored_captures} captures`,
      });
    } else {
      setReplayLabHealth({
        status: 'offline',
        text: 'Replay Lab: offline',
      });
    }

    // 2. Stale
    if (staleRes.status === 'fulfilled') {
      const h = staleRes.value;
      const modeText = h.verification_mode === 'llm' ? 'llm mode' : 'heuristic mode';
      setStaleHealth({
        status: h.status === 'healthy' ? 'healthy' : 'degraded',
        text: `Stale-Answer Detector: healthy · ${modeText}`,
      });
    } else {
      setStaleHealth({
        status: 'offline',
        text: 'Stale-Answer Detector: offline',
      });
    }

    // 3. Insights
    if (insightsRes.status === 'fulfilled') {
      const ok = insightsRes.value;
      setInsightsHealth({
        status: ok ? 'healthy' : 'offline',
        text: ok ? 'Insights: healthy' : 'Insights: offline',
      });
    } else {
      setInsightsHealth({
        status: 'offline',
        text: 'Insights: offline',
      });
    }
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
            ⌘K
          </kbd>
        </button>

        {/* 3 Health dots 8px apart */}
        <div className="flex items-center gap-2">
          <Tooltip content={replayLabHealth.text}>
            <span
              className={`w-2 h-2 rounded-full cursor-help ${getDotColor(
                replayLabHealth.status
              )}`}
            />
          </Tooltip>
          <Tooltip content={staleHealth.text}>
            <span
              className={`w-2 h-2 rounded-full cursor-help ${getDotColor(
                staleHealth.status
              )}`}
            />
          </Tooltip>
          <Tooltip content={insightsHealth.text}>
            <span
              className={`w-2 h-2 rounded-full cursor-help ${getDotColor(
                insightsHealth.status
              )}`}
            />
          </Tooltip>
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

