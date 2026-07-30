import type { CSSProperties } from 'react';
import { SleeperLeagueFilters } from '../types/models';

interface LeagueFilterBarProps {
  filters: SleeperLeagueFilters;
  onChange: (filters: SleeperLeagueFilters) => void;
  txType?: string;
  onTxTypeChange?: (type: string) => void;
  showSuperflexFilter?: boolean;
}

const LEAGUE_SIZES = [
  { value: '', label: 'Any' },
  { value: '8', label: '8' },
  { value: '10', label: '10' },
  { value: '12', label: '12' },
  { value: '14', label: '14' },
];
const SCORING_FORMATS = [
  { value: '', label: 'Any' },
  { value: 'standard', label: 'Standard' },
  { value: 'half_ppr', label: 'Half-PPR' },
  { value: 'ppr', label: 'PPR' },
];
const DRAFT_TYPES = [
  { value: '', label: 'Any' },
  { value: 'snake', label: 'Snake' },
  { value: 'auction', label: 'Auction' },
  { value: 'linear', label: 'Linear' },
];
const LEAGUE_TYPES = [
  { value: '', label: 'Any' },
  { value: 'redraft', label: 'Redraft' },
  { value: 'keeper', label: 'Keeper' },
  { value: 'dynasty', label: 'Dynasty' },
];
const TX_TYPES = [
  { value: '', label: 'All' },
  { value: 'trade', label: 'Trade' },
  { value: 'waiver', label: 'Waiver' },
  { value: 'free_agent', label: 'Free agent' },
];
const SUPERFLEX_OPTIONS = [
  { value: '', label: 'Any' },
  { value: 'true', label: 'Superflex' },
  { value: 'false', label: '1QB' },
];

function pillClassName(active: boolean) {
  return [
    'px-2.5 py-1 text-xs rounded-full border transition-colors cursor-pointer select-none',
    active ? 'font-medium' : 'hover:border-[var(--action-primary)]',
  ].join(' ');
}

function pillStyle(active: boolean): CSSProperties {
  return active
    ? {
        backgroundColor: 'var(--action-primary)',
        borderColor: 'var(--action-primary)',
        color: 'var(--action-on-primary)',
      }
    : {
        backgroundColor: 'var(--surface-raised)',
        borderColor: 'var(--border-subtle)',
        color: 'var(--text-secondary)',
      };
}

export interface PillGroupProps {
  label: string;
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
}

export function PillGroup({ label, options, value, onChange }: PillGroupProps) {
  return (
    <div className="flex items-center gap-1.5 flex-wrap">
      <span className="text-xs font-medium mr-0.5" style={{ color: 'var(--text-muted)' }}>{label}:</span>
      {options.map(opt => (
        <button
          key={opt.value}
          type="button"
          className={pillClassName(value === opt.value)}
          style={pillStyle(value === opt.value)}
          onClick={() => onChange(opt.value)}
          aria-pressed={value === opt.value}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

export default function LeagueFilterBar({
  filters,
  onChange,
  txType,
  onTxTypeChange,
  showSuperflexFilter,
}: LeagueFilterBarProps) {
  const hasFilters =
    !!filters.league_size ||
    !!filters.scoring_format ||
    !!filters.draft_type ||
    !!filters.league_type ||
    !!filters.superflex ||
    !!txType;

  function set(key: keyof SleeperLeagueFilters, value: string) {
    onChange({ ...filters, [key]: value || undefined });
  }

  return (
    <div
      className="flex flex-col gap-2.5 rounded-lg border px-4 py-3"
      style={{ backgroundColor: 'var(--surface-sunken)', borderColor: 'var(--border-subtle)' }}
    >
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium" style={{ color: 'var(--text-secondary)' }}>Filters</span>
        {hasFilters && (
          <button
            className="text-xs hover:underline"
            style={{ color: 'var(--action-primary)' }}
            onClick={() => {
              onChange({});
              onTxTypeChange?.('');
            }}
          >
            Clear all
          </button>
        )}
      </div>

      <div className="flex flex-wrap gap-x-6 gap-y-2">
        {onTxTypeChange && (
          <PillGroup
            label="Type"
            options={TX_TYPES}
            value={txType ?? ''}
            onChange={onTxTypeChange}
          />
        )}

        <PillGroup
          label="Size"
          options={LEAGUE_SIZES}
          value={filters.league_size ?? ''}
          onChange={v => set('league_size', v)}
        />

        <PillGroup
          label="Scoring"
          options={SCORING_FORMATS}
          value={filters.scoring_format ?? ''}
          onChange={v => set('scoring_format', v)}
        />

        <PillGroup
          label="Draft"
          options={DRAFT_TYPES}
          value={filters.draft_type ?? ''}
          onChange={v => set('draft_type', v)}
        />

        <PillGroup
          label="League"
          options={LEAGUE_TYPES}
          value={filters.league_type ?? ''}
          onChange={v => set('league_type', v)}
        />

        {showSuperflexFilter && (
          <PillGroup
            label="Format"
            options={SUPERFLEX_OPTIONS}
            value={filters.superflex ?? ''}
            onChange={v => set('superflex', v)}
          />
        )}
      </div>
    </div>
  );
}
