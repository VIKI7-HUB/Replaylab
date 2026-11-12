import React, { useEffect, useState } from 'react';
import { Command } from 'cmdk';
import { useNavigate } from 'react-router-dom';
import { MagnifyingGlassIcon } from '@radix-ui/react-icons';
import { useTheme } from './theme';
import { searchGlobal, SearchResultItem } from '../api/insights';
import { formatRelativeTime } from '../lib/format';
import { consoleHref, isLocalPath } from './consoles';

export interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({ open, onOpenChange }) => {
  const navigate = useNavigate();
  const { toggleTheme } = useTheme();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResultItem[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if ((e.key === 'k' && (e.metaKey || e.ctrlKey)) || (e.key === '/' && !open)) {
        e.preventDefault();
        onOpenChange(!open);
      }
    };
    document.addEventListener('keydown', down);
    return () => document.removeEventListener('keydown', down);
  }, [open, onOpenChange]);

  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      return;
    }

    const timer = setTimeout(async () => {
      setLoading(true);
      try {
        const res = await searchGlobal(query);
        setResults(res);
      } catch {
        setResults([]);
      } finally {
        setLoading(false);
      }
    }, 150);

    return () => clearTimeout(timer);
  }, [query]);

  if (!open) return null;

  // Paths served by the other console leave this app.
  const go = (path: string) => {
    if (isLocalPath(path)) navigate(path);
    else window.location.href = consoleHref(path);
  };

  const handleSelect = (action: () => void) => {
    action();
    onOpenChange(false);
  };

  return (
    <div className="fixed inset-0 z-50 bg-[var(--overlay)] flex justify-center pt-[15vh]">
      <div
        className="fixed inset-0"
        onClick={() => onOpenChange(false)}
      />
      <div className="relative z-10 w-full max-w-[560px] max-h-[440px] bg-bg rounded-md shadow-float border border-border flex flex-col overflow-hidden">
        <Command label="Command Palette" className="flex flex-col h-full">
          <div className="h-11 px-3 border-b border-border flex items-center gap-2 shrink-0">
            <MagnifyingGlassIcon className="w-[15px] h-[15px] text-text-3 shrink-0" />
            <Command.Input
              value={query}
              onValueChange={setQuery}
              placeholder="Search or jump to…"
              className="w-full bg-transparent text-[13px] text-text placeholder:text-text-3 outline-none"
            />
          </div>

          <Command.List className="overflow-y-auto p-2 max-h-[360px]">
            <Command.Empty className="py-6 text-center text-[13px] text-text-3">
              {loading ? 'Searching…' : 'No results found.'}
            </Command.Empty>

            {results.length > 0 && (
              <Command.Group heading="Results" className="text-[12px] font-medium text-text-3 px-2 py-1.5">
                {results.map((item) => (
                  <Command.Item
                    key={`${item.kind}-${item.id}`}
                    onSelect={() => handleSelect(() => go(item.url))}
                    className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center justify-between cursor-pointer data-[selected=true]:bg-bg-selected"
                  >
                    <div className="flex items-center gap-2 truncate">
                      <span className="text-[12px] text-text-3 shrink-0">{item.kind}</span>
                      <span className="truncate">{item.title}</span>
                    </div>
                    <span className="text-[12px] text-text-3 shrink-0">
                      {formatRelativeTime(item.at)}
                    </span>
                  </Command.Item>
                ))}
              </Command.Group>
            )}

            <Command.Group heading="Go to" className="text-[12px] font-medium text-text-3 px-2 py-1.5">
              <Command.Item
                onSelect={() => handleSelect(() => go('/replaylab/captures'))}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                Captures
              </Command.Item>
              <Command.Item
                onSelect={() => handleSelect(() => go('/replaylab/experiments'))}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                Experiments
              </Command.Item>
              <Command.Item
                onSelect={() => handleSelect(() => go('/replaylab/settings'))}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                Settings
              </Command.Item>
              <Command.Item
                onSelect={() => handleSelect(() => go('/stale/check'))}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                Check an answer
              </Command.Item>
              <Command.Item
                onSelect={() => handleSelect(() => go('/stale/history'))}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                History
              </Command.Item>
            </Command.Group>

            <Command.Group heading="Actions" className="text-[12px] font-medium text-text-3 px-2 py-1.5">
              <Command.Item
                onSelect={() => handleSelect(() => go('/replaylab/experiments/new'))}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                New experiment
              </Command.Item>
              <Command.Item
                onSelect={() => handleSelect(() => go('/stale/check'))}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                Check an answer
              </Command.Item>
              <Command.Item
                onSelect={() => handleSelect(toggleTheme)}
                className="h-9 px-2 rounded-sm text-[13px] text-text flex items-center cursor-pointer data-[selected=true]:bg-bg-selected"
              >
                Toggle theme
              </Command.Item>
            </Command.Group>
          </Command.List>
        </Command>
      </div>
    </div>
  );
};

