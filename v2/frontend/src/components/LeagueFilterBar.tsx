import { SleeperLeagueFilters } from '../types/models';

interface LeagueFilterBarProps {
  filters: SleeperLeagueFilters;
  onChange: (filters: SleeperLeagueFilters) => void;
  txType?: string;
  onTxTypeChange?: (type: string) => void;
}

const LEAGUE_SIZES = ['', '8', '10', '12', '14'];
const SCORING_FORMATS = [
  { value: '', label: 'Any scoring' },
  { value: 'standard', label: 'Standard' },
  { value: 'half_ppr', label: 'Half-PPR' },
  { value: 'ppr', label: 'PPR' },
];
const DRAFT_TYPES = [
  { value: '', label: 'Any draft type' },
  { value: 'snake', label: 'Snake' },
  { value: 'auction', label: 'Auction' },
  { value: 'linear', label: 'Linear' },
];
const TX_TYPES = [
  { value: '', label: 'All types' },
  { value: 'trade', label: 'Trade' },
  { value: 'waiver', label: 'Waiver' },
  { value: 'free_agent', label: 'Free agent' },
];

const selectClass =
  'px-3 py-1.5 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md text-gray-700 dark:text-gray-200 focus:outline-none focus:ring-2 focus:ring-blue-500';

export default function LeagueFilterBar({
  filters,
  onChange,
  txType,
  onTxTypeChange,
}: LeagueFilterBarProps) {
  const hasFilters =
    !!filters.league_size || !!filters.scoring_format || !!filters.draft_type || !!txType;

  function set(key: keyof SleeperLeagueFilters, value: string) {
    onChange({ ...filters, [key]: value || undefined });
  }

  return (
    <div className="flex flex-wrap items-center gap-3 bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 rounded-lg px-4 py-3">
      <span className="text-sm font-medium text-gray-600 dark:text-gray-400 mr-1">Filter:</span>

      {onTxTypeChange && (
        <select
          className={selectClass}
          value={txType ?? ''}
          onChange={e => onTxTypeChange(e.target.value)}
          aria-label="Transaction type"
        >
          {TX_TYPES.map(t => (
            <option key={t.value} value={t.value}>{t.label}</option>
          ))}
        </select>
      )}

      <select
        className={selectClass}
        value={filters.league_size ?? ''}
        onChange={e => set('league_size', e.target.value)}
        aria-label="League size"
      >
        <option value="">Any size</option>
        {LEAGUE_SIZES.filter(Boolean).map(s => (
          <option key={s} value={s}>{s}-team</option>
        ))}
      </select>

      <select
        className={selectClass}
        value={filters.scoring_format ?? ''}
        onChange={e => set('scoring_format', e.target.value)}
        aria-label="Scoring format"
      >
        {SCORING_FORMATS.map(f => (
          <option key={f.value} value={f.value}>{f.label}</option>
        ))}
      </select>

      <select
        className={selectClass}
        value={filters.draft_type ?? ''}
        onChange={e => set('draft_type', e.target.value)}
        aria-label="Draft type"
      >
        {DRAFT_TYPES.map(d => (
          <option key={d.value} value={d.value}>{d.label}</option>
        ))}
      </select>

      {hasFilters && (
        <button
          className="text-sm text-blue-600 dark:text-blue-400 hover:underline ml-auto"
          onClick={() => {
            onChange({});
            onTxTypeChange?.('');
          }}
        >
          Clear filters
        </button>
      )}
    </div>
  );
}
