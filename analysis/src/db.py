"""Postgres access for the valuation pipeline.

Two databases, deliberately:

* ARCHIVE_DATABASE_URL — complete Sleeper history (leagues, drafts, picks,
  transactions). The cloud database only keeps a hot window, so model INPUTS
  that reach back to the draft have to come from here.
* DATABASE_URL — cloud. Player identities, finalized weekly scoring, run
  state, and every `player_valuations` write, so output stays available to the
  API.

Both URLs are read from analysis/.env (or the process environment). No
function here commits; callers own the transaction so a run is all-or-nothing.
"""

import os
import zlib
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import date, datetime
from pathlib import Path

import pandas as pd
import psycopg
from dotenv import load_dotenv

from .config import MIN_ADP_DRAFTS, SeasonDates, Segment, week_ts
from .models import (
    AverageDraftPosition,
    PlayerBeliefState,
    PlayerProfile,
    RunState,
    Trade,
    WeeklyScore,
)
from .parsing import parse_trade

# DATABASE_URL / ARCHIVE_DATABASE_URL come from analysis/.env
load_dotenv(Path(__file__).parent.parent / ".env", override=False)

FANTASY_POSITIONS = ("QB", "RB", "WR", "TE", "K", "DEF")

CLOUD_URL_ENV = "DATABASE_URL"
ARCHIVE_URL_ENV = "ARCHIVE_DATABASE_URL"

# First key of the two-int advisory lock: a private namespace for this job, so
# it can't collide with a lock some other part of the system takes.
ADVISORY_LOCK_NAMESPACE = 0x46465356  # "FFSV"


class MissingPlayerIdentities(RuntimeError):
    """Referenced Sleeper player IDs have no row in cloud `sleeper_players`.

    Hard failure by design: a valuation row can't satisfy player_valuations'
    foreign key without its cloud player record, and silently dropping the
    player (or half a trade) would distort every later belief.
    """

    def __init__(self, missing: list[str]) -> None:
        self.missing = sorted(missing)
        preview = ", ".join(self.missing[:10])
        more = "" if len(self.missing) <= 10 else f" (+{len(self.missing) - 10} more)"
        super().__init__(
            f"{len(self.missing)} player id(s) missing from cloud sleeper_players:"
            f" {preview}{more} — sync player metadata, then rerun"
        )


@dataclass(frozen=True)
class DataSources:
    """The two open connections a replay run reads from."""

    archive: psycopg.Connection
    cloud: psycopg.Connection


@dataclass(frozen=True)
class Inputs:
    """Normalized, window-bounded model inputs — what gets staged to Parquet."""

    adp: list[AverageDraftPosition]
    trades: list[Trade]
    scores: list[WeeklyScore]
    # Every player referenced by any input, including ones that only appear
    # inside a trade. The Valuator needs these to avoid minting nameless
    # DEFAULT beliefs; staged so --from-bundle resolves identities identically.
    players: dict[str, PlayerProfile]
    skipped_trades: int  # rows that parsed to None (picks/FAAB/not two-sided)


def connect(env_var: str) -> psycopg.Connection:
    url = os.environ.get(env_var)
    if not url:
        raise RuntimeError(f"{env_var} is not set — refusing to run")
    conn = psycopg.connect(url)
    with conn.cursor() as cur:
        cur.execute("SET TIME ZONE 'UTC'")  # naive-UTC convention end-to-end
    conn.commit()
    return conn


def get_connection() -> psycopg.Connection:
    """The cloud connection (output + player metadata + finalized scores)."""
    return connect(CLOUD_URL_ENV)


@contextmanager
def open_sources() -> Iterator[DataSources]:
    """Open archive + cloud together, closing both on the way out.

    Both URLs are validated before either connection is made, so a missing one
    fails before any work starts.
    """
    for env_var in (ARCHIVE_URL_ENV, CLOUD_URL_ENV):
        if not os.environ.get(env_var):
            raise RuntimeError(f"{env_var} is not set — refusing to run")

    archive = connect(ARCHIVE_URL_ENV)
    try:
        cloud = connect(CLOUD_URL_ENV)
    except Exception:
        archive.close()
        raise
    try:
        yield DataSources(archive=archive, cloud=cloud)
    finally:
        cloud.close()
        archive.close()


@contextmanager
def read_only_snapshot(conn: psycopg.Connection) -> Iterator[psycopg.Connection]:
    """A read-only REPEATABLE READ transaction: every query in the block sees
    one consistent point in time, and nothing in the block can write."""
    with conn.cursor() as cur:
        cur.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY")
    try:
        yield conn
    finally:
        conn.rollback()  # read-only: nothing to keep


# ------------------------------------------------------------------ locking --


def try_advisory_lock(conn: psycopg.Connection, key: str) -> bool:
    """Session-level lock so a manual replay can't overlap the timer.

    Session-scoped (not transaction-scoped) because the run spans several
    transactions; it survives the commits in between.
    """
    with conn.cursor() as cur:
        cur.execute(
            "SELECT pg_try_advisory_lock(%s, %s)",
            (ADVISORY_LOCK_NAMESPACE, _lock_key(key)),
        )
        got = bool(cur.fetchone()[0])
    conn.commit()
    return got


def advisory_unlock(conn: psycopg.Connection, key: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "SELECT pg_advisory_unlock(%s, %s)",
            (ADVISORY_LOCK_NAMESPACE, _lock_key(key)),
        )
    conn.commit()


def _lock_key(key: str) -> int:
    """Stable signed 32-bit key from a segment string (pg's second lock int)."""
    return zlib.crc32(key.encode()) - 2**31


# ------------------------------------------------------------------ inputs --


def rows_to_scores(rows) -> list[WeeklyScore]:
    return [
        WeeklyScore(week=r[0], player_id=r[1], position=r[2], points=float(r[3]))
        for r in rows
    ]


def get_adp_picks(
    archive: psycopg.Connection, segment: Segment, season: str
) -> list[tuple[str, float]]:
    """Mean pick_no per player across the segment's completed snake drafts.

    Archive-only: it cannot join cloud `sleeper_players`, so this returns bare
    (player_id, adp) pairs and identity resolution happens against the cloud
    (see resolve_players).

    Players drafted in fewer than MIN_ADP_DRAFTS drafts are excluded: a mean
    over a couple of drafts is a fluke, not a market price."""
    sql = """
        SELECT dp.sleeper_player_id, AVG(dp.pick_no) AS adp
        FROM sleeper_draft_picks dp
        JOIN sleeper_drafts d   ON d.sleeper_draft_id = dp.sleeper_draft_id
        JOIN sleeper_leagues l  ON l.sleeper_league_id = d.sleeper_league_id
        WHERE l.ppr = %s AND l.is_superflex = %s AND l.total_rosters = %s
          AND l.league_type = %s
          AND d.type = %s AND d.status = 'complete' AND d.season = %s
          AND dp.sleeper_player_id IS NOT NULL
        GROUP BY dp.sleeper_player_id
        HAVING COUNT(*) >= %s
    """
    with archive.cursor() as cur:
        cur.execute(
            sql,
            (
                segment.ppr,
                segment.is_superflex,
                segment.total_rosters,
                segment.league_type,
                segment.draft_type,
                season,
                MIN_ADP_DRAFTS,
            ),
        )
        return [(r[0], float(r[1])) for r in cur.fetchall()]


def get_trades(
    archive: psycopg.Connection,
    segment: Segment,
    season: str,
    start: datetime,
    end: datetime,
) -> tuple[list[Trade], int]:
    """Completed two-sided player trades in segment leagues, within [start, end).

    Returns the parsed trades plus the count of rows the parser rejected
    (draft picks, FAAB, or not exactly two rosters) — reported, not silently
    dropped.
    """
    sql = """
        SELECT t.sleeper_transaction_id, t.created_at_sleeper,
               t.adds, t.draft_picks, t.waiver_budget
        FROM sleeper_transactions t
        JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id
        WHERE t.type = 'trade' AND t.status = 'complete'
          AND l.ppr = %s AND l.is_superflex = %s AND l.total_rosters = %s
          AND l.league_type = %s AND l.season = %s
          AND t.created_at_sleeper >= %s AND t.created_at_sleeper < %s
        ORDER BY t.created_at_sleeper
    """
    with archive.cursor() as cur:
        cur.execute(
            sql,
            (
                segment.ppr,
                segment.is_superflex,
                segment.total_rosters,
                segment.league_type,
                season,
                _to_ms(start),
                _to_ms(end),
            ),
        )
        rows = cur.fetchall()
    parsed = [parse_trade(r[0], r[1], r[2], r[3], r[4]) for r in rows]
    trades = [t for t in parsed if t is not None]
    return trades, len(parsed) - len(trades)


EPOCH = datetime(1970, 1, 1)


def _to_ms(ts: datetime) -> int:
    """Naive-UTC datetime -> Sleeper's unix-millisecond created_at.

    The exact inverse of parsing.ms_to_dt. Note this deliberately does NOT use
    datetime.timestamp(), which interprets a naive value as local time.
    """
    return int((ts.replace(tzinfo=None) - EPOCH).total_seconds() * 1000)


def get_weekly_scores(
    cloud: psycopg.Connection,
    season: str,
    season_dates: SeasonDates,
    start: datetime,
    end: datetime,
) -> list[WeeklyScore]:
    """PPR points for finalized weeks whose model timestamp is in [start, end).

    Server-side filters cut to the season's finalized, fantasy-position rows;
    the window itself is applied in Python because a week's model timestamp is
    derived (season start + lag), not a stored column.
    """
    sql = """
        SELECT s.week, s.sleeper_player_id, p.position, s.pts_ppr
        FROM sleeper_player_week_stats s
        JOIN sleeper_week_stat_fetches f
             ON f.season = s.season AND f.week = s.week AND f.finalized
        JOIN sleeper_players p ON p.sleeper_player_id = s.sleeper_player_id
        WHERE s.season = %s AND s.pts_ppr IS NOT NULL
          AND p.position = ANY(%s)
        ORDER BY s.week
    """
    with cloud.cursor() as cur:
        cur.execute(sql, (season, list(FANTASY_POSITIONS)))
        scores = rows_to_scores(cur.fetchall())
    return [s for s in scores if start <= week_ts(season_dates, s.week) < end]


def resolve_players(
    cloud: psycopg.Connection, player_ids: list[str]
) -> dict[str, PlayerProfile]:
    """Bulk-resolve names/positions from cloud `sleeper_players`.

    Raises MissingPlayerIdentities if any requested ID has no cloud row.
    """
    ids = sorted(set(player_ids))
    if not ids:
        return {}
    sql = """
        SELECT sleeper_player_id, full_name, position
        FROM sleeper_players
        WHERE sleeper_player_id = ANY(%s)
    """
    with cloud.cursor() as cur:
        cur.execute(sql, (ids,))
        found = {
            r[0]: PlayerProfile(player_id=r[0], name=r[1] or "", position=r[2] or "")
            for r in cur.fetchall()
        }
    missing = [pid for pid in ids if pid not in found]
    if missing:
        raise MissingPlayerIdentities(missing)
    return found


def load_inputs(
    sources: DataSources,
    segment: Segment,
    season: str,
    season_dates: SeasonDates,
    start: datetime,
    end: datetime,
) -> Inputs:
    """Read every model input, then resolve identities as one preflight check.

    Archive and cloud each get their own read-only snapshot; identity
    resolution covers the union of players referenced by ADP, trades, and
    scores, and fails before any output transaction opens.
    """
    with read_only_snapshot(sources.archive) as archive:
        picks = get_adp_picks(archive, segment, season)
        trades, skipped_trades = get_trades(archive, segment, season, start, end)

    with read_only_snapshot(sources.cloud) as cloud:
        scores = get_weekly_scores(cloud, season, season_dates, start, end)

        referenced: set[str] = {pid for pid, _ in picks}
        for t in trades:
            referenced.update(t.side_a)
            referenced.update(t.side_b)
        referenced.update(s.player_id for s in scores)

        profiles = resolve_players(cloud, sorted(referenced))

    adp = [
        AverageDraftPosition(
            player_id=pid,
            player_name=profiles[pid].name,
            position=profiles[pid].position,
            adp=mean_pick,
        )
        for pid, mean_pick in sorted(picks)
        if profiles[pid].position in FANTASY_POSITIONS
    ]
    return Inputs(
        adp=adp,
        trades=trades,
        scores=scores,
        players=profiles,
        skipped_trades=skipped_trades,
    )


# --------------------------------------------------------- state + output --


def load_state(conn: psycopg.Connection, segment_key: str) -> list[PlayerBeliefState]:
    sql = """
        SELECT sleeper_player_id, guess, var, games, cum_par, position, name
        FROM valuation_state WHERE segment = %s
    """
    with conn.cursor() as cur:
        cur.execute(sql, (segment_key,))
        return [
            PlayerBeliefState(
                player_id=r[0], guess=r[1], var=r[2], games=r[3],
                cum_par=r[4], position=r[5] or "DEFAULT", name=r[6] or "",
            )
            for r in cur.fetchall()
        ]


def save_state(
    conn: psycopg.Connection, segment_key: str, states: list[PlayerBeliefState]
) -> None:
    """Full replace: the in-memory Valuator is the source of truth."""
    with conn.cursor() as cur:
        cur.execute("DELETE FROM valuation_state WHERE segment = %s", (segment_key,))
        cur.executemany(
            """
            INSERT INTO valuation_state
                (segment, sleeper_player_id, guess, var, games, cum_par,
                 position, name, updated_at)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, now())
            """,
            [
                (segment_key, s.player_id, s.guess, s.var, s.games, s.cum_par,
                 s.position, s.name)
                for s in states
            ],
        )


def get_run(
    conn: psycopg.Connection, segment_key: str, season: str
) -> RunState | None:
    sql = """
        SELECT last_event_ts, last_transaction_created, last_week_processed
        FROM valuation_runs WHERE segment = %s AND season = %s
    """
    with conn.cursor() as cur:
        cur.execute(sql, (segment_key, season))
        row = cur.fetchone()
    if row is None:
        return None
    last_event_ts = row[0].replace(tzinfo=None) if row[0] is not None else None
    return RunState(
        segment=segment_key, season=season, last_event_ts=last_event_ts,
        last_transaction_created=row[1], last_week_processed=row[2],
    )


def save_run(conn: psycopg.Connection, run: RunState) -> None:
    sql = """
        INSERT INTO valuation_runs
            (segment, season, last_event_ts, last_transaction_created,
             last_week_processed, last_run_at)
        VALUES (%s, %s, %s, %s, %s, now())
        ON CONFLICT (segment, season) DO UPDATE SET
            last_event_ts = EXCLUDED.last_event_ts,
            last_transaction_created = EXCLUDED.last_transaction_created,
            last_week_processed = EXCLUDED.last_week_processed,
            last_run_at = now()
    """
    with conn.cursor() as cur:
        cur.execute(
            sql,
            (run.segment, run.season, run.last_event_ts,
             run.last_transaction_created, run.last_week_processed),
        )


def write_snapshot(
    conn: psycopg.Connection,
    segment_key: str,
    valuation_date: date,
    rankings: pd.DataFrame,
) -> None:
    """rankings = Valuator.rankings(): index is rank, columns include
    player_id, pos, pos_rank, value, vorp, sd, games."""
    sql = """
        INSERT INTO player_valuations
            (segment, sleeper_player_id, valuation_date, rank, pos_rank,
             value, vorp, sd, games, position)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (segment, sleeper_player_id, valuation_date) DO UPDATE SET
            rank = EXCLUDED.rank, pos_rank = EXCLUDED.pos_rank,
            value = EXCLUDED.value, vorp = EXCLUDED.vorp, sd = EXCLUDED.sd,
            games = EXCLUDED.games, position = EXCLUDED.position
    """
    with conn.cursor() as cur:
        cur.executemany(
            sql,
            [
                (segment_key, row.player_id, valuation_date, rank,
                 int(row.pos_rank), float(row.value), float(row.vorp),
                 float(row.sd), float(row.games), row.pos)
                for rank, row in zip(rankings.index, rankings.itertuples(index=False))
            ],
        )


def delete_snapshots(
    conn: psycopg.Connection, segment_key: str, start: date, end: date
) -> None:
    with conn.cursor() as cur:
        cur.execute(
            """
            DELETE FROM player_valuations
            WHERE segment = %s AND valuation_date BETWEEN %s AND %s
            """,
            (segment_key, start, end),
        )


def latest_snapshot_date(
    conn: psycopg.Connection, segment_key: str
) -> date | None:
    """Newest published valuation date for a segment, or None if there is none.

    Used to reject an `--end` that would leave stale rows beyond the range this
    run rewrites.
    """
    with conn.cursor() as cur:
        cur.execute(
            "SELECT max(valuation_date) FROM player_valuations WHERE segment = %s",
            (segment_key,),
        )
        row = cur.fetchone()
    return row[0] if row else None


__all__ = [
    "ARCHIVE_URL_ENV",
    "CLOUD_URL_ENV",
    "DataSources",
    "Inputs",
    "MissingPlayerIdentities",
    "PlayerProfile",
    "advisory_unlock",
    "connect",
    "delete_snapshots",
    "get_adp_picks",
    "get_connection",
    "get_run",
    "get_trades",
    "get_weekly_scores",
    "latest_snapshot_date",
    "load_inputs",
    "load_state",
    "open_sources",
    "read_only_snapshot",
    "resolve_players",
    "rows_to_scores",
    "save_run",
    "save_state",
    "try_advisory_lock",
    "write_snapshot",
]
