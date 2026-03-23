import { useState, useRef, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import type { Model } from '../lib/api';

type ModelDropdownProps = {
  models: Model[];
  value: string | null;
  onChange: (modelId: string) => void;
  placeholder: string;
  disabledIds?: string[];
  disabled?: boolean;
  variant?: 'dashed' | 'solid';
};

type ModelGroup = {
  provider: string;
  models: Model[];
};

type PanelPos = {
  top: number;
  left: number;
  width: number;
  openUp: boolean;
};

function groupAndSort(models: Model[], search: string): ModelGroup[] {
  const query = search.trim().toLowerCase();

  const filtered = query
    ? models.filter((m) => {
        const haystack = `${m.name} ${m.provider}`.toLowerCase();
        return query.split(/\s+/).every((token) => haystack.includes(token));
      })
    : models;

  const providerMap = new Map<string, Model[]>();
  for (const model of filtered) {
    const key = model.provider || 'Other';
    if (!providerMap.has(key)) providerMap.set(key, []);
    providerMap.get(key)!.push(model);
  }

  const sortedProviders = Array.from(providerMap.keys()).sort((a, b) =>
    a.localeCompare(b)
  );

  return sortedProviders.map((provider) => ({
    provider,
    models: providerMap.get(provider)!.sort((a, b) =>
      a.name.localeCompare(b.name)
    ),
  }));
}

const PANEL_MAX_HEIGHT = 360; // search (~60) + list (~280) + padding
const PANEL_WIDTH = 300;
const GAP = 6;

export default function ModelDropdown({
  models,
  value,
  onChange,
  placeholder,
  disabledIds = [],
  disabled = false,
  variant = 'dashed',
}: ModelDropdownProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [panelPos, setPanelPos] = useState<PanelPos | null>(null);

  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  // Calculate where the panel should be positioned (fixed, in viewport coords)
  const calcPos = useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    const spaceAbove = rect.top;
    const spaceBelow = window.innerHeight - rect.bottom;
    const openUp = spaceAbove > spaceBelow || spaceBelow < PANEL_MAX_HEIGHT + GAP;

    setPanelPos({
      left: Math.min(rect.left, window.innerWidth - PANEL_WIDTH - 8),
      top: openUp
        ? rect.top - Math.min(PANEL_MAX_HEIGHT, spaceAbove - GAP)
        : rect.bottom + GAP,
      width: PANEL_WIDTH,
      openUp,
    });
  }, []);

  // Auto-focus search input when panel opens
  useEffect(() => {
    if (isOpen) {
      calcPos();
      setTimeout(() => searchRef.current?.focus(), 0);
    }
  }, [isOpen, calcPos]);

  // Reposition on scroll/resize while open
  useEffect(() => {
    if (!isOpen) return;
    window.addEventListener('scroll', calcPos, true);
    window.addEventListener('resize', calcPos);
    return () => {
      window.removeEventListener('scroll', calcPos, true);
      window.removeEventListener('resize', calcPos);
    };
  }, [isOpen, calcPos]);

  // Click-outside to close (checks both trigger and panel)
  useEffect(() => {
    if (!isOpen) return;
    function handleMouseDown(e: MouseEvent) {
      const target = e.target as Node;
      if (
        triggerRef.current?.contains(target) ||
        panelRef.current?.contains(target)
      ) return;
      setIsOpen(false);
      setSearch('');
    }
    document.addEventListener('mousedown', handleMouseDown);
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [isOpen]);

  function close() {
    setIsOpen(false);
    setSearch('');
  }

  function handleSelect(modelId: string) {
    onChange(modelId);
    close();
  }

  const groups = groupAndSort(models, search);

  const triggerLabel =
    variant === 'solid'
      ? (models.find((m) => m.id === value)?.name ?? placeholder)
      : placeholder;

  const triggerClass = [
    'fusion-dropdown-trigger',
    variant === 'solid' ? 'fusion-dropdown-trigger--solid' : '',
    isOpen ? 'open' : '',
  ]
    .filter(Boolean)
    .join(' ');

  const panel =
    isOpen && panelPos
      ? createPortal(
          <div
            ref={panelRef}
            className={`fusion-dropdown-panel${panelPos.openUp ? ' fusion-dropdown-panel--up' : ' fusion-dropdown-panel--down'}`}
            style={{
              position: 'fixed',
              top: panelPos.top,
              left: panelPos.left,
              width: panelPos.width,
              zIndex: 9999,
            }}
          >
            <div className="fusion-dropdown-search-wrap">
              <input
                ref={searchRef}
                type="text"
                className="fusion-dropdown-search"
                placeholder="Search models..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') close();
                }}
              />
            </div>
            <div className="fusion-dropdown-list">
              {groups.length === 0 && (
                <div className="fusion-dropdown-empty">No models found</div>
              )}
              {groups.map(({ provider, models: providerModels }) => (
                <div key={provider}>
                  <div className="fusion-dropdown-group-header">{provider}</div>
                  {providerModels.map((model) => {
                    const isDisabled = disabledIds.includes(model.id);
                    return (
                      <button
                        key={model.id}
                        type="button"
                        className={`fusion-dropdown-option${isDisabled ? ' fusion-dropdown-option--disabled' : ''}`}
                        onClick={() => !isDisabled && handleSelect(model.id)}
                        disabled={isDisabled}
                      >
                        {model.name}
                      </button>
                    );
                  })}
                </div>
              ))}
            </div>
          </div>,
          document.body
        )
      : null;

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className={triggerClass}
        onClick={() => {
          if (!disabled) setIsOpen((o) => !o);
        }}
        disabled={disabled}
      >
        <span>{triggerLabel}</span>
        <svg
          className="fusion-dropdown-chevron"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>

      {panel}
    </>
  );
}
