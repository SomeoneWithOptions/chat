import { useEffect, useRef, useState } from 'react';
import { type Model } from '../lib/api';

type ModelSelectorProps = {
  models: Model[];
  selectedModelId: string;
  onSelectModel: (modelId: string) => void;
  favoriteModelIds: Set<string>;
  onToggleFavorite: (modelId: string) => void;
  showAllModels: boolean;
  onToggleShowAll: (show: boolean) => void;
  disabled?: boolean;
};

function formatPrice(micros: number): string {
  if (micros <= 0) return 'Free';
  const dollars = micros / 1_000_000;
  if (dollars < 0.001) return `$${dollars.toFixed(6)}`;
  return `$${dollars.toFixed(4)}`;
}

export default function ModelSelector({
  models,
  selectedModelId,
  onSelectModel,
  favoriteModelIds,
  onToggleFavorite,
  showAllModels,
  onToggleShowAll,
  disabled,
}: ModelSelectorProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const wrapperRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const selectedModel = models.find((m) => m.id === selectedModelId);

  const filteredModels = models.filter((model) => {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    return (
      model.name.toLowerCase().includes(q) ||
      model.id.toLowerCase().includes(q) ||
      model.provider.toLowerCase().includes(q)
    );
  });

  useEffect(() => {
    if (isOpen && searchRef.current) {
      searchRef.current.focus();
    }
  }, [isOpen]);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setIsOpen(false);
        setSearch('');
      }
    }
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [isOpen]);

  return (
    <div className="model-selector-wrapper" ref={wrapperRef}>
      <button
        className={`model-selector-trigger ${isOpen ? 'open' : ''}`}
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        type="button"
      >
        <span className="model-selector-name">
          {selectedModel ? selectedModel.name : 'Select model'}
        </span>
        <svg className="model-selector-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>

      {isOpen && (
        <>
          <div className="dropdown-backdrop" onClick={() => { setIsOpen(false); setSearch(''); }} />
          <div className="model-selector-dropdown">
            <div className="model-dropdown-header">
              <input
                ref={searchRef}
                type="text"
                className="model-search"
                placeholder="Search models..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              <button
                type="button"
                className={`model-dropdown-toggle ${showAllModels ? 'active' : ''}`}
                onClick={() => onToggleShowAll(!showAllModels)}
                role="switch"
                aria-checked={showAllModels}
                aria-label="Show all models"
              >
                <span className={`toggle-track ${showAllModels ? 'active' : ''}`} aria-hidden="true">
                  <span className="toggle-thumb" />
                </span>
                <span className="model-dropdown-toggle-label">All</span>
              </button>
            </div>

            <div className="model-dropdown-list">
              {filteredModels.length === 0 && (
                <div style={{ padding: '16px', textAlign: 'center', fontSize: '12px', color: 'var(--text-tertiary)' }}>
                  No models found
                </div>
              )}

              {filteredModels.map((model) => (
                <button
                  key={model.id}
                  className={`model-option ${model.id === selectedModelId ? 'selected' : ''}`}
                  onClick={() => {
                    onSelectModel(model.id);
                    setIsOpen(false);
                    setSearch('');
                  }}
                  type="button"
                >
                  <div className="model-option-info">
                    <div className="model-option-name">
                      {favoriteModelIds.has(model.id) && <span className="model-option-star">&#9733;</span>}
                      {model.name}
                    </div>
                    <div className="model-option-meta">
                      {model.provider} &middot; {model.contextWindow.toLocaleString()} ctx &middot; {formatPrice(model.promptPriceMicrosUsd)}/tok
                    </div>
                  </div>
                  <button
                    className={`model-option-fav ${favoriteModelIds.has(model.id) ? 'favorited' : ''}`}
                    onClick={(e) => {
                      e.stopPropagation();
                      onToggleFavorite(model.id);
                    }}
                    type="button"
                    aria-label={favoriteModelIds.has(model.id) ? 'Remove from favorites' : 'Add to favorites'}
                  >
                    {favoriteModelIds.has(model.id) ? '\u2605' : '\u2606'}
                  </button>
                </button>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
