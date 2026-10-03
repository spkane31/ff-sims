import logging
from datetime import datetime

from espn_api.football import League
from temporalio import activity

from activities.teams import ESPNLeagueSyncParams
from db import get_connection, resolve_league_id

logger = logging.getLogger(__name__)


def _upsert_player(cur, espn_id: int, name: str, position: str) -> int:
    # Atomic upsert (not SELECT-then-INSERT) to avoid a concurrent-insert deadlock; no-op DO UPDATE keeps first-writer-wins semantics.
    cur.execute(
        "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
        "VALUES (%s, %s, %s, 'active', NOW(), NOW()) "
        "ON CONFLICT (espn_id) WHERE espn_id <> 0 DO UPDATE SET espn_id = players.espn_id "
        "RETURNING id",
        (espn_id, name, position),
    )
    return cur.fetchone()[0]


@activity.defn
def fetch_and_upsert_schedule(params: ESPNLeagueSyncParams) -> None:
    logger.info(
        "fetch_and_upsert_schedule START league=%s year=%d",
        params.espn_league_id,
        params.year,
    )
    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
    )
    logger.info(
        "League loaded: current_week=%d year=%d", league.current_week, league.year
    )

    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)
        with conn.cursor() as cur:
            cur.execute(
                "SELECT espn_id, id FROM teams WHERE league_id = %s", (league_id,)
            )
            team_map = {row[0]: row[1] for row in cur.fetchall()}
        logger.info(
            "Team map loaded: %d teams for league_id=%d", len(team_map), league_id
        )

        # ESPN leaves a matchup's winner UNDECIDED until its scoring period is
        # final, so it is the only signal that separates live scores from final
        # ones — totals are published all week long.
        outcomes = {team.team_id: team.outcomes for team in league.teams}

        with conn.cursor() as cur:
            last_week = 17
            if datetime.now().year == league.year:
                # Future playoff opponents are not yet determined, but ESPN
                # already publishes the entire regular-season schedule.
                last_week = max(league.current_week, league.settings.reg_season_count)

            for week in range(1, last_week + 1):
                activity.heartbeat(f"week {week}")
                logger.info(
                    "Processing schedule week %d/%d for league %s year %d",
                    week,
                    last_week,
                    params.espn_league_id,
                    params.year,
                )
                # box_scores() clamps future weeks to the current week. Use
                # scoreboard() for actual future opponents, also for pre-2019
                # seasons where box_scores() is unsupported.
                entries = (
                    league.scoreboard(week=week)
                    if league.year < 2019 or week > league.current_week
                    else league.box_scores(week=week)
                )

                for bs in entries:
                    if not hasattr(bs, "home_team") or not hasattr(bs, "away_team"):
                        logger.debug(
                            "Skipping box score with no home/away_team attr: %r", bs
                        )
                        continue
                    if not bs.home_team or not bs.away_team:
                        logger.debug(
                            "Skipping box score week %d — home=%r away=%r",
                            week,
                            bs.home_team,
                            bs.away_team,
                        )
                        continue

                    home_db_id = team_map.get(bs.home_team.team_id)
                    away_db_id = team_map.get(bs.away_team.team_id)
                    if home_db_id is None or away_db_id is None:
                        logger.warning(
                            "Skipping matchup week %d — team not found in DB", week
                        )
                        continue

                    home_score = getattr(bs, "home_score", 0)
                    away_score = getattr(bs, "away_score", 0)
                    home_proj = getattr(bs, "home_projected", -1)
                    away_proj = getattr(bs, "away_projected", -1)
                    team_outcomes = outcomes.get(bs.home_team.team_id) or []
                    decided = (
                        week <= len(team_outcomes) and team_outcomes[week - 1] != "U"
                    )
                    completed = decided and home_score > 0 and away_score > 0

                    cur.execute(
                        "SELECT id FROM matchups WHERE league_id = %s AND week = %s AND year = %s "
                        "AND home_team_id = %s AND away_team_id = %s",
                        (league_id, week, league.year, home_db_id, away_db_id),
                    )
                    existing = cur.fetchone()

                    if existing is None:
                        cur.execute(
                            "INSERT INTO matchups (league_id, week, year, home_team_id, away_team_id, "
                            "home_team_final_score, away_team_final_score, "
                            "home_team_espn_projected_score, away_team_espn_projected_score, "
                            "completed, is_playoff, game_type, created_at, updated_at) "
                            "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,NOW(),NOW()) RETURNING id",
                            (
                                league_id,
                                week,
                                league.year,
                                home_db_id,
                                away_db_id,
                                home_score,
                                away_score,
                                home_proj,
                                away_proj,
                                completed,
                                bs.is_playoff,
                                getattr(bs, "matchup_type", "REGULAR"),
                            ),
                        )
                        matchup_id = cur.fetchone()[0]
                    else:
                        matchup_id = existing[0]
                        cur.execute(
                            "UPDATE matchups SET home_team_final_score = %s, away_team_final_score = %s, "
                            "home_team_espn_projected_score = %s, away_team_espn_projected_score = %s, "
                            "completed = %s, updated_at = NOW() WHERE id = %s",
                            (
                                home_score,
                                away_score,
                                home_proj,
                                away_proj,
                                completed,
                                matchup_id,
                            ),
                        )

                    if (
                        completed
                        and hasattr(bs, "home_lineup")
                        and hasattr(bs, "away_lineup")
                    ):
                        for player, team_db_id in [
                            (p, home_db_id) for p in bs.home_lineup
                        ] + [(p, away_db_id) for p in bs.away_lineup]:
                            player_db_id = _upsert_player(
                                cur,
                                player.playerId,
                                player.name,
                                getattr(player, "position", "Unknown"),
                            )
                            cur.execute(
                                "SELECT id FROM box_scores WHERE matchup_id = %s AND player_id = %s AND team_id = %s",
                                (matchup_id, player_db_id, team_db_id),
                            )
                            if cur.fetchone() is None:
                                started = player.slot_position not in ("BE", "IR")
                                cur.execute(
                                    "INSERT INTO box_scores (matchup_id, player_id, team_id, slot_position, "
                                    "actual_points, projected_points, started_flag, created_at, updated_at) "
                                    "VALUES (%s,%s,%s,%s,%s,%s,%s,NOW(),NOW())",
                                    (
                                        matchup_id,
                                        player_db_id,
                                        team_db_id,
                                        player.slot_position,
                                        player.points,
                                        player.projected_points,
                                        started,
                                    ),
                                )

                # Commit per week, not once per season — shorter transactions avoid cross-league deadlocks on shared players.
                conn.commit()


@activity.defn
def mark_schedule_fetched(espn_league_id: str) -> None:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE espn_league_credentials SET last_schedule_fetched_at = NOW() "
                "WHERE espn_league_id = %s",
                (espn_league_id,),
            )
        conn.commit()
