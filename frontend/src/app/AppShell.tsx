import React, { useEffect, useState } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { TopBar } from './TopBar';
import { CommandPalette } from './CommandPalette';
import { EngineGate } from './EngineGate';
import { PageErrorBoundary } from './PageErrorBoundary';

export const AppShell: React.FC = () => {
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  // Below md the sidebar is a slide-out menu; on larger screens it is always shown.
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const { pathname } = useLocation();

  // Close the menu after moving to another page.
  useEffect(() => setMobileNavOpen(false), [pathname]);
  // A capture's drawer opens over the Captures page, so it is the same page.
  const pageKey = pathname.startsWith('/replaylab/captures') ? '/replaylab/captures' : pathname;

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-bg text-text">
      {/* Sidebar */}
      <Sidebar mobileOpen={mobileNavOpen} />
      {mobileNavOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 md:hidden"
          aria-hidden="true"
          onClick={() => setMobileNavOpen(false)}
        />
      )}

      {/* Main Container */}
      <div className="flex-1 min-w-0 flex flex-col h-full overflow-hidden">
        {/* TopBar 48px */}
        <TopBar
          onOpenSearch={() => setCommandPaletteOpen(true)}
          onOpenMenu={() => setMobileNavOpen(true)}
        />

        {/* Page Content: pt-8 px-6 pb-6 (§8.3 page padding: 24px all sides, 32px top) */}
        <main className="flex-1 overflow-y-auto overflow-x-hidden pt-5 px-4 pb-5 md:pt-8 md:px-6 md:pb-6">
          <EngineGate>
            <PageErrorBoundary resetKey={pathname}>
              <div key={pageKey} className="animate-fade-in">
                <Outlet />
              </div>
            </PageErrorBoundary>
          </EngineGate>
        </main>
      </div>

      {/* Global Command Palette */}
      <CommandPalette
        open={commandPaletteOpen}
        onOpenChange={setCommandPaletteOpen}
      />
    </div>
  );
};

