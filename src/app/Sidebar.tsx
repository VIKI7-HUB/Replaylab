import React, { useState, useEffect } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { DoubleArrowLeftIcon, DoubleArrowRightIcon } from '@radix-ui/react-icons';
import { Tooltip } from '../components/Tooltip';
import { productHealth } from '../api/health';
import { CONSOLE_NAME, Product, productOfPath } from './consoles';

const NAV: { product: Product; label: string; items: { to: string; label: string }[] }[] = [
  {
    product: 'replaylab',
    label: 'Replay Lab',
    items: [
      { to: '/replaylab/captures', label: 'Captures' },
      { to: '/replaylab/experiments', label: 'Experiments' },
      { to: '/replaylab/settings', label: 'Settings' },
    ],
  },
  {
    product: 'stale',
    label: 'Stale answers',
    items: [
      { to: '/stale/check', label: 'Check' },
      { to: '/stale/history', label: 'History' },
    ],
  },
];

export const Sidebar: React.FC = () => {
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    return localStorage.getItem('sidebar_collapsed') === 'true';
  });
  const [version, setVersion] = useState<string>('v1.0.0');

  // The version shown is that of the product whose page is open.
  const product: Product = productOfPath(useLocation().pathname) ?? 'replaylab';

  useEffect(() => {
    localStorage.setItem('sidebar_collapsed', String(collapsed));
  }, [collapsed]);

  useEffect(() => {
    // Retry until the service answers: it may still be starting (see EngineGate).
    let timer: ReturnType<typeof setTimeout>;
    let cancelled = false;
    const load = () =>
      productHealth[product]().then((h) => {
        if (cancelled) return;
        if (h.version) setVersion(`v${h.version}`);
        else timer = setTimeout(load, 5000);
      });
    load();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [product]);

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
          <span className="text-[14px] font-semibold text-text truncate">{CONSOLE_NAME}</span>
        </div>

        {/* Navigation list */}
        <nav className="flex flex-col pt-2">
          {NAV.map((group) => (
            <React.Fragment key={group.product}>
              <div className="text-[12px] font-medium text-text-3 px-4 pt-4 pb-1">{group.label}</div>
              {group.items.map((item) => (
                <NavLink key={item.to} to={item.to} className={navItemClass}>
                  {item.label}
                </NavLink>
              ))}
            </React.Fragment>
          ))}
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
