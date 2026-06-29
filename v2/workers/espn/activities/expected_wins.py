import logging
import random
from dataclasses import dataclass

from temporalio import activity

from db import get_connection, resolve_league_id

logger = logging.getLogger(__name__)

NUM_SIMULATIONS = 10_000


@dataclass
class ExpectedWinsParams:
    espn_league_id: str
    year: int


# ---------------------------------------------------------------------------
# Core simulation helpers (pure functions, no DB)
# ---------------------------------------------------------------------------

def _extract_team_weekly_scores(
    matchups: list[dict],
) -> tuple[dict[int, dict[int, float]], list[int]]:
    """Return {team_id: {week: score}} and sorted weeks for completed regular season games."""
    team_scores: dict[int, dict[int, float]] = {}
    weeks_set: set[int] = set()
    for m in matchups:
        if not m["completed"] or m["is_playoff"] or m["game_type"] != "NONE":
            continue
        home_id, away_id, week = m["home_team_id"], m["away_team_id"], m["week"]
        team_scores.setdefault(home_id, {})[week] = float(m["home_team_final_score"])
        team_scores.setdefault(away_id, {})[week] = float(m["away_team_final_score"])
        weeks_set.add(week)
    return team_scores, sorted(weeks_set)


def _run_monte_carlo(
    team_scores: dict[int, dict[int, float]],
    weeks: list[int],
    num_sims: int = NUM_SIMULATIONS,
) -> dict[int, float]:
    """
    Random-schedule Monte Carlo: for each simulation shuffle teams into random
    pairings per week and accumulate win totals.  Mirrors Go's runScheduleSimulations.
    """
    team_ids = list(team_scores.keys())
    if not team_ids or len(team_ids) % 2 != 0:
        return {}

    totals: dict[int, float] = {t: 0.0 for t in team_ids}
    shuffled = team_ids[:]

    for _ in range(num_sims):
        for week in weeks:
            random.shuffle(shuffled)
            for i in range(0, len(shuffled) - 1, 2):
                t1, t2 = shuffled[i], shuffled[i + 1]
                s1 = team_scores[t1].get(week)
                s2 = team_scores[t2].get(week)
                if s1 is not None and s2 is not None:
                    if s1 > s2:
                        totals[t1] += 1
                    elif s2 > s1:
                        totals[t2] += 1

    return totals


def _calculate_actual_stats(
    matchups: list[dict],
) -> dict[int, tuple[int, int, int]]:
    """Return {team_id: (wins, losses, games)} for completed regular season matchups."""
    stats: dict[int, tuple[int, int, int]] = {}
    for m in matchups:
        if not m["completed"] or m["is_playoff"] or m["game_type"] != "NONE":
            continue
        home_id, away_id = m["home_team_id"], m["away_team_id"]
        hw, hl, hg = stats.get(home_id, (0, 0, 0))
        aw, al, ag = stats.get(away_id, (0, 0, 0))
        if m["home_team_final_score"] > m["away_team_final_score"]:
            hw += 1
            al += 1
        elif m["away_team_final_score"] > m["home_team_final_score"]:
            aw += 1
            hl += 1
        stats[home_id] = (hw, hl, hg + 1)
        stats[away_id] = (aw, al, ag + 1)
    return stats


def _calculate_sos(
    all_matchups: list[dict],
    actual_stats: dict[int, tuple[int, int, int]],
) -> dict[int, float]:
    """
    Average opponent win rate for each team, over the full season schedule.
    Uses actual win rates for opponents with completed games, 0.5 for future
    opponents with no games played yet.  Mirrors Go's calculateStrengthOfSchedule.
    """
    sos: dict[int, float] = {}
    opp_counts: dict[int, int] = {}

    for m in all_matchups:
        if m["is_playoff"] or m["game_type"] != "NONE":
            continue
        home_id, away_id = m["home_team_id"], m["away_team_id"]

        # Home team's SOS contribution: away team's win rate
        aw, _al, ag = actual_stats.get(away_id, (0, 0, 0))
        if ag > 0:
            sos[home_id] = sos.get(home_id, 0.0) + aw / ag
            opp_counts[home_id] = opp_counts.get(home_id, 0) + 1
        elif not m["completed"]:
            sos[home_id] = sos.get(home_id, 0.0) + 0.5
            opp_counts[home_id] = opp_counts.get(home_id, 0) + 1

        # Away team's SOS contribution: home team's win rate
        hw, _hl, hg = actual_stats.get(home_id, (0, 0, 0))
        if hg > 0:
            sos[away_id] = sos.get(away_id, 0.0) + hw / hg
            opp_counts[away_id] = opp_counts.get(away_id, 0) + 1
        elif not m["completed"]:
            sos[away_id] = sos.get(away_id, 0.0) + 0.5
            opp_counts[away_id] = opp_counts.get(away_id, 0) + 1

    for team_id in sos:
        if opp_counts.get(team_id, 0) > 0:
            sos[team_id] /= opp_counts[team_id]

    return sos


def _weekly_expected_wins_for_week(
    all_matchups: list[dict],
    target_week: int,
    num_sims: int = NUM_SIMULATIONS,
) -> dict[int, float]:
    """
    Run Monte Carlo using only target_week scores, returning each team's
    win probability for that week (a value between 0 and 1).
    Mirrors Go's CalculateWeeklyExpectedWins.
    """
    week_matchups = [
        m for m in all_matchups
        if m["week"] == target_week and m["completed"]
        and not m["is_playoff"] and m["game_type"] == "NONE"
    ]
    if not week_matchups:
        return {}

    team_scores: dict[int, dict[int, float]] = {}
    for m in week_matchups:
        home_id, away_id = m["home_team_id"], m["away_team_id"]
        team_scores.setdefault(home_id, {})[target_week] = float(m["home_team_final_score"])
        team_scores.setdefault(away_id, {})[target_week] = float(m["away_team_final_score"])

    raw = _run_monte_carlo(team_scores, [target_week], num_sims)
    return {t: raw[t] / num_sims for t in raw}


# ---------------------------------------------------------------------------
# DB write helpers
# ---------------------------------------------------------------------------

def _upsert_weekly_expected_wins(
    conn,
    team_id: int,
    week: int,
    year: int,
    league_id: int,
    cumulative_ew: float,
    weekly_ew: float,
    cumulative_el: float,
    cumulative_actual_wins: int,
    cumulative_actual_losses: int,
    weekly_win: bool,
    sos: float,
    team_score: float,
    opp_score: float,
    opponent_id: int,
) -> None:
    with conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO weekly_expected_wins (
                team_id, week, year, league_id,
                expected_wins, weekly_expected_wins,
                expected_losses, weekly_expected_losses,
                actual_wins, actual_losses, weekly_actual_win,
                strength_of_schedule, weekly_win_probability,
                team_score, opponent_score, opponent_team_id, point_differential,
                created_at, updated_at
            ) VALUES (
                %s,%s,%s,%s,
                %s,%s,
                %s,%s,
                %s,%s,%s,
                %s,%s,
                %s,%s,%s,%s,
                NOW(),NOW()
            )
            ON CONFLICT (team_id, week, year) DO UPDATE SET
                expected_wins            = EXCLUDED.expected_wins,
                weekly_expected_wins     = EXCLUDED.weekly_expected_wins,
                expected_losses          = EXCLUDED.expected_losses,
                weekly_expected_losses   = EXCLUDED.weekly_expected_losses,
                actual_wins              = EXCLUDED.actual_wins,
                actual_losses            = EXCLUDED.actual_losses,
                weekly_actual_win        = EXCLUDED.weekly_actual_win,
                strength_of_schedule     = EXCLUDED.strength_of_schedule,
                weekly_win_probability   = EXCLUDED.weekly_win_probability,
                team_score               = EXCLUDED.team_score,
                opponent_score           = EXCLUDED.opponent_score,
                opponent_team_id         = EXCLUDED.opponent_team_id,
                point_differential       = EXCLUDED.point_differential,
                updated_at               = NOW()
            """,
            (
                team_id, week, year, league_id,
                cumulative_ew, weekly_ew,
                cumulative_el, 1.0 - weekly_ew,
                cumulative_actual_wins, cumulative_actual_losses, weekly_win,
                sos, weekly_ew,
                team_score, opp_score, opponent_id, team_score - opp_score,
            ),
        )


def _get_prev_week_cumulative(conn, team_id: int, year: int, week: int) -> tuple[float, float, int, int]:
    """Return (expected_wins, expected_losses, actual_wins, actual_losses) from week-1 record."""
    if week <= 1:
        return 0.0, 0.0, 0, 0
    with conn.cursor() as cur:
        cur.execute(
            "SELECT expected_wins, expected_losses, actual_wins, actual_losses "
            "FROM weekly_expected_wins WHERE team_id = %s AND year = %s AND week = %s",
            (team_id, year, week - 1),
        )
        row = cur.fetchone()
    if row is None:
        return 0.0, 0.0, 0, 0
    return float(row[0]), float(row[1]), int(row[2]), int(row[3])


# ---------------------------------------------------------------------------
# Per-week and season processing
# ---------------------------------------------------------------------------

def _process_week(
    conn,
    all_matchups: list[dict],
    league_id: int,
    year: int,
    week: int,
) -> None:
    """Compute and upsert weekly_expected_wins for all teams in a given week."""
    week_matchups = [
        m for m in all_matchups
        if m["week"] == week and m["completed"]
        and not m["is_playoff"] and m["game_type"] == "NONE"
    ]
    if not week_matchups:
        return

    # Actual stats through this week (for SOS)
    through_week = [m for m in all_matchups if m["week"] <= week]
    actual_stats = _calculate_actual_stats(through_week)

    # SOS uses full season schedule + current actual stats
    sos_map = _calculate_sos(all_matchups, actual_stats)

    # Weekly expected wins per team (0-1 win probability for this week)
    weekly_ew_map = _weekly_expected_wins_for_week(all_matchups, week)

    for m in week_matchups:
        for team_id, is_home in (
            (m["home_team_id"], True),
            (m["away_team_id"], False),
        ):
            if is_home:
                team_score = float(m["home_team_final_score"])
                opp_score = float(m["away_team_final_score"])
                opponent_id = m["away_team_id"]
            else:
                team_score = float(m["away_team_final_score"])
                opp_score = float(m["home_team_final_score"])
                opponent_id = m["home_team_id"]

            weekly_win = team_score > opp_score
            weekly_ew = weekly_ew_map.get(team_id, 0.0)
            sos = sos_map.get(team_id, 0.0)

            prev_ew, prev_el, prev_wins, prev_losses = _get_prev_week_cumulative(
                conn, team_id, year, week
            )

            _upsert_weekly_expected_wins(
                conn,
                team_id=team_id,
                week=week,
                year=year,
                league_id=league_id,
                cumulative_ew=prev_ew + weekly_ew,
                weekly_ew=weekly_ew,
                cumulative_el=prev_el + (1.0 - weekly_ew),
                cumulative_actual_wins=prev_wins + (1 if weekly_win else 0),
                cumulative_actual_losses=prev_losses + (0 if weekly_win else 1),
                weekly_win=weekly_win,
                sos=sos,
                team_score=team_score,
                opp_score=opp_score,
                opponent_id=opponent_id,
            )


def _finalize_season(
    conn,
    all_matchups: list[dict],
    league_id: int,
    year: int,
    final_week: int,
) -> None:
    """Upsert season_expected_wins from the final week's cumulative records."""
    # Get all team IDs that have weekly data for this year
    with conn.cursor() as cur:
        cur.execute(
            "SELECT DISTINCT team_id FROM weekly_expected_wins "
            "WHERE league_id = %s AND year = %s",
            (league_id, year),
        )
        team_ids = [row[0] for row in cur.fetchall()]

    if not team_ids:
        logger.info("No weekly expected wins data to finalize for league %d year %d", league_id, year)
        return

    for team_id in team_ids:
        # Read final week's cumulative record
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT expected_wins, expected_losses, actual_wins, actual_losses,
                       strength_of_schedule, week
                FROM weekly_expected_wins
                WHERE team_id = %s AND year = %s
                ORDER BY week DESC LIMIT 1
                """,
                (team_id, year),
            )
            row = cur.fetchone()

        if row is None:
            continue

        exp_wins, exp_losses, act_wins, act_losses, sos, last_week = (
            float(row[0]), float(row[1]), int(row[2]), int(row[3]), float(row[4]), int(row[5])
        )

        # Points for / against from matchup data
        total_pf = total_pa = 0.0
        games_played = 0
        for m in all_matchups:
            if not m["completed"] or m["is_playoff"] or m["game_type"] != "NONE":
                continue
            if m["home_team_id"] == team_id:
                total_pf += float(m["home_team_final_score"])
                total_pa += float(m["away_team_final_score"])
                games_played += 1
            elif m["away_team_id"] == team_id:
                total_pf += float(m["away_team_final_score"])
                total_pa += float(m["home_team_final_score"])
                games_played += 1

        avg_pf = total_pf / games_played if games_played > 0 else 0.0
        avg_pa = total_pa / games_played if games_played > 0 else 0.0

        # Playoff participation
        playoff_made = any(
            m["game_type"] != "NONE"
            and (m["home_team_id"] == team_id or m["away_team_id"] == team_id)
            for m in all_matchups
        )

        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO season_expected_wins (
                    team_id, year, league_id, final_week,
                    expected_wins, expected_losses, actual_wins, actual_losses,
                    strength_of_schedule,
                    total_points_for, total_points_against,
                    average_points_for, average_points_against,
                    playoff_made, final_standing,
                    created_at, updated_at
                ) VALUES (
                    %s,%s,%s,%s,
                    %s,%s,%s,%s,
                    %s,
                    %s,%s,
                    %s,%s,
                    %s,%s,
                    NOW(),NOW()
                )
                ON CONFLICT (team_id, year) DO UPDATE SET
                    final_week              = EXCLUDED.final_week,
                    expected_wins           = EXCLUDED.expected_wins,
                    expected_losses         = EXCLUDED.expected_losses,
                    actual_wins             = EXCLUDED.actual_wins,
                    actual_losses           = EXCLUDED.actual_losses,
                    strength_of_schedule    = EXCLUDED.strength_of_schedule,
                    total_points_for        = EXCLUDED.total_points_for,
                    total_points_against    = EXCLUDED.total_points_against,
                    average_points_for      = EXCLUDED.average_points_for,
                    average_points_against  = EXCLUDED.average_points_against,
                    playoff_made            = EXCLUDED.playoff_made,
                    final_standing          = EXCLUDED.final_standing,
                    updated_at              = NOW()
                """,
                (
                    team_id, year, league_id, last_week,
                    exp_wins, exp_losses, act_wins, act_losses,
                    sos,
                    total_pf, total_pa,
                    avg_pf, avg_pa,
                    playoff_made, 0,
                ),
            )


# ---------------------------------------------------------------------------
# Activities
# ---------------------------------------------------------------------------

@activity.defn
def get_matchup_years(espn_league_id: str) -> list[int]:
    """Return all years that have matchup data for the given ESPN league."""
    with get_connection() as conn:
        league_id = resolve_league_id(conn, espn_league_id)
        with conn.cursor() as cur:
            cur.execute(
                "SELECT DISTINCT year FROM matchups "
                "WHERE league_id = %s AND game_type = 'NONE' "
                "ORDER BY year",
                (league_id,),
            )
            return [row[0] for row in cur.fetchall()]


@activity.defn
def calculate_and_store_expected_wins(params: ExpectedWinsParams) -> None:
    """
    Run Monte Carlo expected-wins simulation for a league/year and persist
    results to weekly_expected_wins and season_expected_wins.  Idempotent.
    """
    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)

        with conn.cursor() as cur:
            cur.execute(
                "SELECT home_team_id, away_team_id, week, year, "
                "home_team_final_score, away_team_final_score, "
                "completed, is_playoff, game_type "
                "FROM matchups WHERE league_id = %s AND year = %s AND game_type = 'NONE' "
                "ORDER BY week",
                (league_id, params.year),
            )
            rows = cur.fetchall()

        if not rows:
            logger.info(
                "No matchups for league %s year %d — skipping expected wins",
                params.espn_league_id, params.year,
            )
            return

        matchups = [
            {
                "home_team_id": r[0], "away_team_id": r[1], "week": r[2], "year": r[3],
                "home_team_final_score": r[4], "away_team_final_score": r[5],
                "completed": r[6], "is_playoff": r[7], "game_type": r[8],
            }
            for r in rows
        ]

        last_completed_week = max(
            (m["week"] for m in matchups if m["completed"]), default=0
        )
        if last_completed_week == 0:
            logger.info(
                "No completed weeks for league %s year %d — skipping",
                params.espn_league_id, params.year,
            )
            return

        logger.info(
            "Calculating expected wins: league=%s year=%d weeks=1-%d",
            params.espn_league_id, params.year, last_completed_week,
        )

        for week in range(1, last_completed_week + 1):
            activity.heartbeat(f"year={params.year} week={week}")
            logger.info("  week %d/%d", week, last_completed_week)
            _process_week(conn, matchups, league_id, params.year, week)

        _finalize_season(conn, matchups, league_id, params.year, last_completed_week)
        conn.commit()

        logger.info(
            "Expected wins complete: league=%s year=%d",
            params.espn_league_id, params.year,
        )
