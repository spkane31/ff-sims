import { PillGroup } from './LeagueFilterBar';
import { SleeperADPFilters } from '../types/models';

interface ADPFilterBarProps {
  filters: SleeperADPFilters;
  onChange: (filters: SleeperADPFilters) => void;
  availableSeasons: string[];
  position: string;
  onPositionChange: (position: string) => void;
}

const LEAGUE_SIZES = [
  { value: '8', label: '8' },
  { value: '10', label: '10' },
  { value: '12', label: '12' },
  { value: '14+', label: '14+' },
];
const SCORING_FORMATS = [
  { value: 'standard', label: 'Standard' },
  { value: 'half_ppr', label: 'Half-PPR' },
  { value: 'ppr', label: 'PPR' },
];
const SUPERFLEX_OPTIONS = [
  { value: 'true', label: 'Superflex' },
  { value: 'false', label: '1QB' },
];
const POSITIONS = [
  { value: '', label: 'All' },
  { value: 'QB', label: 'QB' },
  { value: 'RB', label: 'RB' },
  { value: 'WR', label: 'WR' },
  { value: 'TE', label: 'TE' },
  { value: 'K', label: 'K' },
  { value: 'DEF', label: 'DEF' },
];

export default function ADPFilterBar({ filters, onChange, availableSeasons, position, onPositionChange }: ADPFilterBarProps) {
  function set(key: keyof SleeperADPFilters, value: string) {
    onChange({ ...filters, [key]: value });
  }

  const seasonOptions = availableSeasons.map(s => ({ value: s, label: s }));
  const currentSeason = filters.season ?? seasonOptions[0]?.value ?? '';

  return (
    <div className="flex flex-col gap-2.5 bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 rounded-lg px-4 py-3">
      <div className="flex flex-wrap gap-x-6 gap-y-2">
        <PillGroup
          label="Size"
          options={LEAGUE_SIZES}
          value={filters.league_size ?? '12'}
          onChange={v => set('league_size', v)}
        />

        <PillGroup
          label="Scoring"
          options={SCORING_FORMATS}
          value={filters.scoring_format ?? 'ppr'}
          onChange={v => set('scoring_format', v)}
        />

        <PillGroup
          label="Format"
          options={SUPERFLEX_OPTIONS}
          value={filters.superflex ?? 'true'}
          onChange={v => set('superflex', v)}
        />

        {seasonOptions.length > 0 && (
          <PillGroup
            label="Season"
            options={seasonOptions}
            value={currentSeason}
            onChange={v => set('season', v)}
          />
        )}

        <PillGroup
          label="Position"
          options={POSITIONS}
          value={position}
          onChange={onPositionChange}
        />
      </div>
    </div>
  );
}
