import React, { useState, useEffect } from 'react';
import { NavLink } from 'react-router-dom';
import { DoubleArrowLeftIcon, DoubleArrowRightIcon } from '@radix-ui/react-icons';
import { Tooltip } from '../components/Tooltip';
import { getHealth } from '../api/replaylab';

export const Sidebar: React.FC = () => {
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    return localStorage.getItem('sidebar_collapsed') === 'true';
  });
  const [version, setVersion] = useState<string>('v1.0.0');

  const appMode = import.meta.env.VITE_APP_MODE || 'all';

  let title = 'Operator console';
  if (appMode === 'replaylab') title = 'Bifrost Replay Lab';
  else if (appMode === 'stale') title = 'Stale-Answer Detector';

  useEffect(() => {
    localStorage.setItem('sidebar_collapsed', String(collapsed));
  }, [collapsed]);

  useEffect(() => {
    getHealth()
      .then((h) => {
        if (h.version) setVersion(`v${h.version}`);
      })
      .catch(() => {
        // fallback
      });
  }, []);

  const navItemClass = ({ isActive }: { isActive: boolean }) =>
    `h-[30px] mx-2 px-2 rounded flex items-center text-[13px] transition-colors select-none ${
      isActive
        ? 'bg-bg-selected text-text font-medium'
        : 'text-text-2 hover:bg-bg-muted hover:text-text'
    }`;

  if (collapsed) {
    return (
      <aside className="w-14 bg-bg-subtle border-r border-border h-screen flex flex-col justify-between py-3 shrink-0">
        <div className="flex justify-center">
          <Tooltip content="Expand sidebar" side="right">
            <button
              type="button"
              onClick={() => setCollapsed(false)}
              aria-label="Expand sidebar"
              className="w-8 h-8 inline-flex items-center justify-center rounded text-text-2 hover:bg-bg-muted hover:text-text transition-colors"
            >
              <DoubleArrowRightIcon className="w-[15px] h-[15px]" />
            </button>
          </Tooltip>
        </div>
      </aside>
    );
  }

  return (
    <aside className="w-[232px] bg-bg-subtle border-r border-border h-screen flex flex-col justify-between shrink-0">
      <div className="flex flex-col">
        {/* Top 56px */}
        <div className="h-14 px-4 flex items-center border-b border-border">
          <span className="text-[14px] font-semibold text-text truncate">{title}</span>
        </div>

        {/* Navigation list */}
        <nav className="flex flex-col pt-2">
          {/* Overview group (all mode) */}
          {appMode === 'all' && (
            <NavLink to="/" end className={navItemClass}>
              Overview
            </NavLink>
          )}

          {/* Replay Lab group (all or replaylab mode) */}
          {(appMode === 'all' || appMode === 'replaylab') && (
            <>
              <div className="text-[12px] font-medium text-text-3 px-4 pt-4 pb-1">
                Replay Lab
              </div>
              <NavLink to="/replaylab/captures" className={navItemClass}>
                Captures
              </NavLink>
              <NavLink to="/replaylab/experiments" className={navItemClass}>
                Experiments
              </NavLink>
              <NavLink to="/replaylab/settings" className={navItemClass}>
                Settings
              </NavLink>
            </>
          )}

          {/* Stale answers group (all or stale mode) */}
          {(appMode === 'all' || appMode === 'stale') && (
            <>
              <div className="text-[12px] font-medium text-text-3 px-4 pt-4 pb-1">
                Stale answers
              </div>
              <NavLink to="/stale/check" className={navItemClass}>
                Check
              </NavLink>
              <NavLink to="/stale/history" className={navItemClass}>
                History
              </NavLink>
            </>
          )}
        </nav>
      </div>

      {/* Bottom version and collapse button */}
      <div className="h-12 px-4 border-t border-border flex items-center justify-between">
        <span className="text-[12px] text-text-3">{version}</span>
        <Tooltip content="Collapse sidebar" side="top">
          <button
            type="button"
            onClick={() => setCollapsed(true)}
            aria-label="Collapse sidebar"
            className="w-7 h-7 inline-flex items-center justify-center rounded text-text-2 hover:bg-bg-muted hover:text-text transition-colors"
          >
            <DoubleArrowLeftIcon className="w-[15px] h-[15px]" />
          </button>
        </Tooltip>
      </div>
    </aside>
  );
};
