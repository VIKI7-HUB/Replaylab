import React, { useState } from 'react';
import { Outlet } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { TopBar } from './TopBar';
import { CommandPalette } from './CommandPalette';

export const AppShell: React.FC = () => {
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-bg text-text">
      {/* Sidebar */}
      <Sidebar />

      {/* Main Container */}
      <div className="flex-1 flex flex-col h-full overflow-hidden">
        {/* TopBar 48px */}
        <TopBar onOpenSearch={() => setCommandPaletteOpen(true)} />

        {/* Page Content: pt-8 px-6 pb-6 (§8.3 page padding: 24px all sides, 32px top) */}
        <main className="flex-1 overflow-y-auto pt-8 px-6 pb-6">
          <Outlet />
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

