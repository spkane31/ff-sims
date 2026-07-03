import { SleeperLeagueFilters } from '../types/models';

interface LeagueFilterBarProps {
  filters: SleeperLeagueFilters;
  onChange: (filters: SleeperLeagueFilters) => void;
  txType?: string;
  onTxTypeChange?: (type: string) => void;
  showPicksFilter?: boolean;
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
const PICKS_OPTIONS = [
  { value: '', label: 'Any' },
  { value: 'true', label: 'Players only' },
];

function pillClass(active: boolean) {
  return [
    'px-2.5 py-1 text-xs rounded-full border transition-colors cursor-pointer select-none',
    active
      ? 'bg-blue-600 border-blue-600 text-white font-medium'
      : 'bg-white dark:bg-gray-700 border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:border-blue-400 dark:hover:border-blue-500',
  ].join(' ');
}

interface PillGroupProps {
  label: string;
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
}

function PillGroup({ label, options, value, onChange }: PillGroupProps) {
  return (
    <div className="flex items-center gap-1.5 flex-wrap">
      <span className="text-xs font-medium text-gray-500 dark:text-gray-400 mr-0.5">{label}:</span>
      {options.map(opt => (
        <button
          key={opt.value}
          type="button"
          className={pillClass(value === opt.value)}
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
  showPicksFilter,
}: LeagueFilterBarProps) {
  const hasFilters =
    !!filters.league_size ||
    !!filters.scoring_format ||
    !!filters.draft_type ||
    !!filters.league_type ||
    !!filters.exclude_picks ||
    !!txType;

  function set(key: keyof SleeperLeagueFilters, value: string) {
    onChange({ ...filters, [key]: value || undefined });
  }

  return (
    <div className="flex flex-col gap-2.5 bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 rounded-lg px-4 py-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-gray-600 dark:text-gray-400">Filters</span>
        {hasFilters && (
          <button
            className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
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

        {showPicksFilter && (
          <PillGroup
            label="Assets"
            options={PICKS_OPTIONS}
            value={filters.exclude_picks ?? ''}
            onChange={v => set('exclude_picks', v)}
          />
        )}
      </div>
    </div>
  );
}
