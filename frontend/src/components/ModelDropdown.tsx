import { useState, useRef, useEffect } from 'react';
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

function groupAndSort(models: Model[], search: string): ModelGroup[] {
  const query = search.trim().toLowerCase();

  const filtered = query
    ? models.filter(
        (m) =>
          m.name.toLowerCase().includes(query) ||
          m.provider.toLowerCase().includes(query)
      )
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
  const wrapperRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  // Auto-focus search input when panel opens
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => searchRef.current?.focus(), 0);
    }
  }, [isOpen]);

  // Click-outside to close
  useEffect(() => {
    if (!isOpen) return;
    function handleMouseDown(e: MouseEvent) {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setIsOpen(false);
        setSearch('');
      }
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
    'council-dropdown-trigger',
    variant === 'solid' ? 'council-dropdown-trigger--solid' : '',
    isOpen ? 'open' : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className="council-dropdown-wrapper" ref={wrapperRef}>
      <button
        type="button"
        className={triggerClass}
        onClick={() => {
          if (!disabled) setIsOpen((o) => !o);
        }}
        disabled={disabled}
      >
        <span>{triggerLabel}</span>
        <svg
          className="council-dropdown-chevron"
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

      {isOpen && (
        <>
          <div
            className="dropdown-backdrop"
            onClick={close}
          />
          <div className="council-dropdown-panel">
            <div className="council-dropdown-search-wrap">
              <input
                ref={searchRef}
                type="text"
                className="council-dropdown-search"
                placeholder="Search models..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') close();
                }}
              />
            </div>
            <div className="council-dropdown-list">
              {groups.length === 0 && (
                <div className="council-dropdown-empty">No models found</div>
              )}
              {groups.map(({ provider, models: providerModels }) => (
                <div key={provider}>
                  <div className="council-dropdown-group-header">{provider}</div>
                  {providerModels.map((model) => {
                    const isDisabled = disabledIds.includes(model.id);
                    return (
                      <button
                        key={model.id}
                        type="button"
                        className={`council-dropdown-option${isDisabled ? ' council-dropdown-option--disabled' : ''}`}
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
          </div>
        </>
      )}
    </div>
  );
}
