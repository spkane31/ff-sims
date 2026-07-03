"""Postgres access for the valuation pipeline.

DATABASE_URL is read from analysis/.env. No function here commits;
callers own the transaction so a run is all-or-nothing.
"""

import os
from datetime import date
from pathlib import Path

import pandas as pd
import psycopg
from dotenv import load_dotenv

from .config import Segment
from .models import (
    AverageDraftPosition,
    PlayerBeliefState,
    RunState,
    Trade,
    WeeklyScore,
)
from .parsing import parse_trade

# DATABASE_URL comes from analysis/.env
load_dotenv(Path(__file__).parent.parent / ".env", override=False)

FANTASY_POSITIONS = ("QB", "RB", "WR", "TE", "K", "DEF")


def get_connection() -> psycopg.Connection:
    conn = psycopg.connect(os.environ["DATABASE_URL"])
    with conn.cursor() as cur:
        cur.execute("SET TIME ZONE 'UTC'")  # naive-UTC convention end-to-end
    return conn


# ---------------------------------------------------------------- inputs --


def rows_to_adp(rows) -> list[AverageDraftPosition]:
    return [
        AverageDraftPosition(
            player_id=r[0], player_name=r[1], position=r[2], adp=float(r[3])
        )
        for r in rows
    ]


def rows_to_scores(rows) -> list[WeeklyScore]:
    return [
        WeeklyScore(week=r[0], player_id=r[1], position=r[2], points=float(r[3]))
        for r in rows
    ]


def get_adp(
    conn: psycopg.Connection, segment: Segment, season: str
) -> list[AverageDraftPosition]:
    """Mean pick_no per player across the segment's completed snake drafts."""
    sql = """
        SELECT dp.sleeper_player_id, p.full_name, p.position,
               AVG(dp.pick_no) AS adp
        FROM sleeper_draft_picks dp
        JOIN sleeper_drafts d   ON d.sleeper_draft_id = dp.sleeper_draft_id
        JOIN sleeper_leagues l  ON l.sleeper_league_id = d.sleeper_league_id
        JOIN sleeper_players p  ON p.sleeper_player_id = dp.sleeper_player_id
        WHERE l.ppr = %s AND l.is_superflex = %s AND l.total_rosters = %s
          AND l.league_type = %s
          AND d.type = %s AND d.status = 'complete' AND d.season = %s
          AND p.position = ANY(%s)
        GROUP BY dp.sleeper_player_id, p.full_name, p.position
    """
    with conn.cursor() as cur:
        cur.execute(
            sql,
            (
                segment.ppr,
                segment.is_superflex,
                segment.total_rosters,
                segment.league_type,
                segment.draft_type,
                season,
                list(FANTASY_POSITIONS),
            ),
        )
        return rows_to_adp(cur.fetchall())


def get_trades(
    conn: psycopg.Connection, segment: Segment, season: str, since_created: int
) -> list[Trade]:
    """Completed two-sided player trades in segment leagues, past the watermark."""
    sql = """
        SELECT t.sleeper_transaction_id, t.created_at_sleeper,
               t.adds, t.draft_picks, t.waiver_budget
        FROM sleeper_transactions t
        JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id
        WHERE t.type = 'trade' AND t.status = 'complete'
          AND l.ppr = %s AND l.is_superflex = %s AND l.total_rosters = %s
          AND l.league_type = %s AND l.season = %s
          AND t.created_at_sleeper > %s
        ORDER BY t.created_at_sleeper
    """
    with conn.cursor() as cur:
        cur.execute(
            sql,
            (
                segment.ppr,
                segment.is_superflex,
                segment.total_rosters,
                segment.league_type,
                season,
                since_created,
            ),
        )
        rows = cur.fetchall()
    trades = [parse_trade(r[0], r[1], r[2], r[3], r[4]) for r in rows]
    return [t for t in trades if t is not None]


def get_weekly_scores(
    conn: psycopg.Connection, season: str, after_week: int
) -> list[WeeklyScore]:
    """PPR points for finalized weeks after the watermark (all NFL players)."""
    sql = """
        SELECT s.week, s.sleeper_player_id, p.position, s.pts_ppr
        FROM sleeper_player_week_stats s
        JOIN sleeper_week_stat_fetches f
             ON f.season = s.season AND f.week = s.week AND f.finalized
        JOIN sleeper_players p ON p.sleeper_player_id = s.sleeper_player_id
        WHERE s.season = %s AND s.week > %s AND s.pts_ppr IS NOT NULL
          AND p.position = ANY(%s)
        ORDER BY s.week
    """
    with conn.cursor() as cur:
        cur.execute(sql, (season, after_week, list(FANTASY_POSITIONS)))
        return rows_to_scores(cur.fetchall())


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
