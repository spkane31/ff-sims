import argparse
import csv
import json
import logging
import os
import time
from datetime import datetime
from typing import List, Dict

from dotenv import find_dotenv, load_dotenv
from espn_api.football import League
import psycopg2


load_dotenv(find_dotenv())

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - [%(pathname)s:%(lineno)d] - %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)


def upsert_team(
    espn_id: int,
    owner: str,
    conn: "psycopg2.connection" = None,
    file_name: str = None,
) -> None:
    logging.info(f"Upserting team: {espn_id} - {owner} (file: {file_name}) (conn: {conn is not None})")
    if conn is not None:
        with conn.cursor() as cur:
            cur.execute("SELECT * FROM teams WHERE espn_id = %s", (espn_id,))
            if cur.fetchone():
                cur.execute("UPDATE teams SET owner = %s WHERE espn_id = %s", (owner, espn_id))
            else:
                cur.execute("INSERT INTO teams (owner, espn_id) VALUES (%s, %s)", (owner, espn_id))
            conn.commit()

    if file_name is not None:
        with open(file_name, "a") as f:
            json.dump({"espn_id": espn_id, "owner": owner}, f)
            f.write("\n")

    return None


def upsert_teams(
    teams: List[Dict[str, any]],
    conn: "psycopg2.connection" = None,
    file_name: str = None,
    year: int = datetime.now().year,
) -> None:
    """Save team data to database and/or file"""
    logging.info(f"Upserting teams to {'database' if conn else 'file'}")

    # Create formatted team data for file output
    team_data = [
        {
            "espn_id": team.team_id,
            "owner": " ".join([team.owners[0]["firstName"], team.owners[0]["lastName"]]),
            "team_name": team.team_name,
            "year": year,
        }
        for team in teams
    ]

    # Update database if connection provided
    if conn is not None:
        for team in teams:
            with conn.cursor() as cur:
                cur.execute("SELECT * FROM teams WHERE espn_id = %s", (team.team_id,))
                if cur.fetchone():
                    cur.execute("UPDATE teams SET owner = %s WHERE espn_id = %s", (team.owner, team.team_id))
                else:
                    cur.execute("INSERT INTO teams (owner, espn_id) VALUES (%s, %s)", (team.owner, team.team_id))
                conn.commit()

    # Write to file if filename provided
    if file_name is not None:
        logging.info(f"Writing {len(team_data)} teams to {file_name}")
        with open(file_name, "w") as f:
            json.dump(team_data, f, indent=2)

    return None


def upsert_matchups(
    league: League,
    conn: "psycopg2.connection" = None,
    file_name: str = "matchups.json",
) -> None:
    if conn is not None:
        for week in range(1, 15):
            for matchup in league.scoreboard(week=week):
                if matchup.matchup_type != "NONE":
                    continue

                upsert_matchup(
                    conn,
                    week,
                    league.year,
                    matchup.home_team.team_id,
                    matchup.away_team.team_id,
                    0,
                    0,
                    0,
                    0,
                    False,
                )

    if file_name is not None:
        matchups = []
        for week in range(1, 18):
            for matchup in league.scoreboard(week=week):
                if matchup.matchup_type != "NONE":
                    continue

                matchups.append(
                    {
                        "week": week,
                        "year": league.year,
                        "home_team_espn_id": matchup.home_team.team_id,
                        "away_team_espn_id": matchup.away_team.team_id,
                        "home_team_final_score": 0,
                        "away_team_final_score": 0,
                        "home_team_espn_projected_score": 0,
                        "away_team_espn_projected_score": 0,
                        "completed": False,
                    }
                )

        with open(file_name, "a") as f:
            json.dump(matchups, f)


def upsert_matchup(
    week: int,
    year: int,
    home_team: int,
    away_team: int,
    home_team_score: float,
    away_team_score: float,
    home_team_projected_score: float,
    away_team_projected_score: float,
    completed: bool,
    conn: "psycopg2.connection" = None,
    file_name: str = "matchups.json",
) -> None:
    if conn is not None:
        with conn.cursor() as cur:
            # Check if the matchup already exists
            cur.execute(
                "SELECT * FROM matchups WHERE week = %s AND year = %s AND home_team_espn_id = %s AND away_team_espn_id = %s",
                (week, year, home_team, away_team),
            )
            if cur.fetchone():
                cur.execute(
                    "UPDATE matchups SET home_team_final_score = %s, away_team_final_score = %s, home_team_espn_projected_score = %s, away_team_espn_projected_score = %s, completed = %s WHERE week = %s AND year = %s AND home_team_espn_id = %s AND away_team_espn_id = %s",
                    (
                        home_team_score,
                        away_team_score,
                        home_team_projected_score,
                        away_team_projected_score,
                        completed,
                        week,
                        year,
                        home_team,
                        away_team,
                    ),
                )
            else:
                cur.execute(
                    "INSERT INTO matchups (week, year, home_team_espn_id, away_team_espn_id, home_team_final_score, away_team_final_score, home_team_espn_projected_score, away_team_espn_projected_score, completed) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)",
                    (
                        week,
                        year,
                        home_team,
                        away_team,
                        home_team_score,
                        away_team_score,
                        home_team_projected_score,
                        away_team_projected_score,
                        completed,
                    ),
                )
            conn.commit()

    if file_name is not None:
        matchup_data = {
            "week": week,
            "year": year,
            "home_team_espn_id": home_team,
            "away_team_espn_id": away_team,
            "home_team_final_score": home_team_score,
            "away_team_final_score": away_team_score,
            "home_team_espn_projected_score": home_team_projected_score,
            "away_team_espn_projected_score": away_team_projected_score,
            "completed": completed,
        }
        with open(file_name, "a") as f:
            json.dump(matchup_data, f)
            f.write("\n")

    return None


def upsert_player_boxscore(
    conn: "psycopg2.connection",
    name: str,
    player_id: int,
    projected_points: float,
    actual_points: float,
    position: str,
    status: str,
    week: int,
    year: int,
    team_id: int,
) -> None:
    # check if the boxscore exists
    with conn.cursor() as cur:
        cur.execute(
            "SELECT * FROM box_score_players WHERE player_id = %s AND week = %s AND year = %s",
            (player_id, week, year),
        )
        if cur.fetchone():
            cur.execute(
                "UPDATE box_score_players SET player_name = %s, projected_points = %s, actual_points = %s, player_position = %s, status = %s WHERE player_id = %s AND week = %s AND year = %s AND owner_espn_id = %s",
                (name, projected_points, actual_points, position, status, player_id, week, year, team_id),
            )
        else:
            cur.execute(
                "INSERT INTO box_score_players (player_name, player_id, projected_points, actual_points, player_position, status, week, year, owner_espn_id) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)",
                (name, player_id, projected_points, actual_points, position, status, week, year, team_id),
            )
        conn.commit()
    return None


def get_schedule(league: League, conn: "psycopg2.connection" = None, file_name: str = None) -> None:
    logging.info(f"Upserting teams for {league.year}")
    teams_file = file_name.replace("matchups", "teams") if file_name else None
    upsert_teams(league.teams, conn=conn, file_name=teams_file, year=league.year)

    logging.info(f"Creating matchups based on {league.year}")
    matchups_data = []
    box_score_data = []

    for week in range(1, 18):
        logging.info(f"Year: {league.year}\tWeek: {week}")
        if week > league.current_week and datetime.now().year == league.year:
            break
        if league.year < 2019:
            for matchup in league.scoreboard(week=week):
                if not hasattr(matchup, "away_team") or not hasattr(matchup, "home_team"):
                    break

                matchup_info = {
                    "week": week,
                    "year": league.year,
                    "game_type": matchup.matchup_type,
                    "is_playoff": matchup.is_playoff,
                    "home_team_espn_id": matchup.home_team.team_id,
                    "away_team_espn_id": matchup.away_team.team_id,
                    "home_team_final_score": matchup.home_score,
                    "away_team_final_score": matchup.away_score,
                    "home_team_espn_projected_score": -1,
                    "away_team_espn_projected_score": -1,
                    "completed": True,
                }

                matchups_data.append(matchup_info)

                # Also update database if needed
                if conn is not None:
                    upsert_matchup(
                        week,
                        league.year,
                        matchup.home_team.team_id,
                        matchup.away_team.team_id,
                        matchup.home_score,
                        matchup.away_score,
                        -1,
                        -1,
                        True,
                        conn=conn,
                    )
        else:
            # box_scores func only works for the current year
            for matchup in league.box_scores(week=week):
                if matchup.away_team == 0 or matchup.home_team == 0:
                    continue

                matchup_info = {
                    "week": week,
                    "year": league.year,
                    "game_type": matchup.matchup_type,
                    "is_playoff": matchup.is_playoff,
                    "home_team_espn_id": matchup.home_team.team_id,
                    "away_team_espn_id": matchup.away_team.team_id,
                    "home_team_final_score": matchup.home_score,
                    "away_team_final_score": matchup.away_score,
                    "home_team_espn_projected_score": matchup.home_projected,
                    "away_team_espn_projected_score": matchup.away_projected,
                    "completed": league.current_week >= week,
                    "home_team_lineup": [
                        {
                            "slot_position": player.slot_position,
                            "points": player.points,
                            "projected_points": player.projected_points,
                            "pro_opponent": player.pro_opponent,
                            "pro_pos_rank": player.pro_pos_rank,
                            "game_played": player.game_played,
                            "game_date": player.game_date.strftime("%Y-%m-%d %H:%M:%S")
                            if hasattr(player, "game_date")
                            else None,
                            "on_bye_week": player.on_bye_week,
                            "active_status": player.active_status,
                            "player_id": player.playerId,
                            "name": player.name,
                            "eligible_slots": player.eligibleSlots,
                            "pro_team": player.proTeam,
                            "on_team_id": player.onTeamId,
                            "injured": player.injured,
                            "injury_status": player.injuryStatus,
                            "percent_owned": player.percent_owned,
                            "percent_started": player.percent_started,
                            "stats": player.stats,
                        }
                        for player in matchup.home_lineup
                    ],
                    "away_team_lineup": [
                        {
                            "slot_position": player.slot_position,
                            "points": player.points,
                            "projected_points": player.projected_points,
                            "pro_opponent": player.pro_opponent,
                            "pro_pos_rank": player.pro_pos_rank,
                            "game_played": player.game_played,
                            "game_date": player.game_date.strftime("%Y-%m-%d %H:%M:%S")
                            if hasattr(player, "game_date")
                            else None,
                            "on_bye_week": player.on_bye_week,
                            "active_status": player.active_status,
                            "player_id": player.playerId,
                            "name": player.name,
                            "eligible_slots": player.eligibleSlots,
                            "pro_team": player.proTeam,
                            "on_team_id": player.onTeamId,
                            "injured": player.injured,
                            "injury_status": player.injuryStatus,
                            "percent_owned": player.percent_owned,
                            "percent_started": player.percent_started,
                            "stats": player.stats,
                        }
                        for player in matchup.away_lineup
                    ],
                }

                matchups_data.append(matchup_info)

                # Also update database if needed
                if conn is not None:
                    upsert_matchup(
                        week,
                        league.year,
                        matchup.home_team.team_id,
                        matchup.away_team.team_id,
                        matchup.home_score,
                        matchup.away_score,
                        matchup.home_projected,
                        matchup.away_projected,
                        league.current_week > week,
                        conn=conn,
                    )

                home_team_id = matchup.home_team.team_id
                away_team_id = matchup.away_team.team_id

                if league.year == datetime.now().year and week < league.current_week:
                    # Collect box score data for players
                    for player in matchup.home_lineup:
                        player_info = {
                            "player_name": player.name,
                            "player_id": player.playerId,
                            "projected_points": player.projected_points,
                            "actual_points": player.points,
                            "player_position": player.position,
                            "status": player.slot_position,
                            "week": week,
                            "year": league.year,
                            "owner_espn_id": home_team_id,
                        }
                        box_score_data.append(player_info)

                        # Update database if needed
                        if conn is not None:
                            upsert_player_boxscore(
                                conn,
                                player.name,
                                player.playerId,
                                player.projected_points,
                                player.points,
                                player.position,
                                player.slot_position,
                                week,
                                league.year,
                                home_team_id,
                            )

                    for player in matchup.away_lineup:
                        player_info = {
                            "player_name": player.name,
                            "player_id": player.playerId,
                            "projected_points": player.projected_points,
                            "actual_points": player.points,
                            "player_position": player.position,
                            "status": player.slot_position,
                            "week": week,
                            "year": league.year,
                            "owner_espn_id": away_team_id,
                        }
                        box_score_data.append(player_info)

                        # Update database if needed
                        if conn is not None:
                            upsert_player_boxscore(
                                conn,
                                player.name,
                                player.playerId,
                                player.projected_points,
                                player.points,
                                player.position,
                                player.slot_position,
                                week,
                                league.year,
                                away_team_id,
                            )

    # Write matchup data to file if filename provided
    if file_name is not None:
        with open(file_name, "w") as f:
            json.dump(matchups_data, f, indent=2)

        # Write box score data to a separate file
        if box_score_data:
            box_score_file = file_name.replace("matchups", "box_score_players")
            with open(box_score_file, "w") as f:
                json.dump(box_score_data, f, indent=2)

    return None


def write_box_score_players_to_db(
    box_score_players: List[Dict[str, any]],
    year: int,
    conn: "psycopg2.connection" = None,
    file_name: str = "box_score_players.json",
) -> None:
    if conn is not None:
        counter = 0
        with conn.cursor() as cur:
            for player in box_score_players:
                cur.execute(
                    "INSERT INTO box_score_players (player_name, player_id, projected_points, actual_points, player_position, status, week, year) SELECT %s, %s, %s, %s, %s, %s, %s, %s WHERE NOT EXISTS (SELECT 1 FROM box_score_players WHERE player_id = %s AND week = %s AND year = %s)",
                    (
                        player["name"],
                        player["id"],
                        player["projection"],
                        player["actual"],
                        player["position"],
                        player["status"],
                        player["week"],
                        year,
                        player["id"],
                        player["week"],
                        year,
                    ),
                )
                counter += 1

                if counter % 100 == 0:
                    conn.commit()

            conn.commit()
            cur.close()

    if file_name is not None:
        with open(file_name, "a") as f:
            json.dump(box_score_players, f)

    return None


def get_simple_draft(
    league: League,
    conn: "psycopg2.connection" = None,
    file_name: str = None,
) -> None:
    """Get draft data and write to database and/or file"""
    logging.info(f"Getting draft data for {league.year}")

    draft_selections = []

    for pick in league.draft:
        try:
            logging.info(f"Processing draft pick: {pick.playerName} (ID: {pick.playerId})")

            player_info = league.player_info(playerId=pick.playerId)

            # Create draft selection record
            draft_data = {
                "player_name": pick.playerName,
                "player_position": player_info.position if player_info else "Unknown",
                "player_id": pick.playerId,
                "round": pick.round_num,
                "pick": pick.round_pick,
                "year": league.year,
                "owner_espn_id": pick.team.team_id,
            }

            draft_selections.append(draft_data)

            # Update database if connection provided
            if conn is not None:
                with conn.cursor() as cur:
                    # Check if selection exists
                    cur.execute(
                        "SELECT 1 FROM draft_selections WHERE player_name = %s AND owner_espn_id = %s AND round = %s AND pick = %s AND year = %s",
                        (
                            pick.playerName,
                            pick.team.team_id,
                            pick.round_num,
                            pick.round_pick,
                            league.year,
                        ),
                    )

                    res = cur.fetchone()
                    if res and res[0] == 1:
                        # Update position if record exists
                        cur.execute(
                            "UPDATE draft_selections SET player_position = %s WHERE player_name = %s AND owner_espn_id = %s AND round = %s AND pick = %s AND year = %s",
                            (
                                player_info.position if player_info else "Unknown",
                                pick.playerName,
                                pick.team.team_id,
                                pick.round_num,
                                pick.round_pick,
                                league.year,
                            ),
                        )
                    else:
                        # Insert new record
                        cur.execute(
                            "INSERT INTO draft_selections (player_name, player_id, player_position, owner_espn_id, round, pick, year) VALUES (%s, %s, %s, %s, %s, %s, %s)",
                            (
                                pick.playerName,
                                pick.playerId,
                                player_info.position if player_info else "Unknown",
                                pick.team.team_id,
                                pick.round_num,
                                pick.round_pick,
                                league.year,
                            ),
                        )

                conn.commit()

            # Avoid rate limiting when fetching player info
            time.sleep(0.1)

        except Exception as e:
            logging.error(f"Error processing draft pick {pick.playerName}: {e}")
            continue

    # Write draft data to file if filename provided
    if file_name is not None and draft_selections:
        logging.info(f"Writing {len(draft_selections)} draft selections to {file_name}")
        with open(file_name, "w") as f:
            json.dump(draft_selections, f, indent=2)

    return None


def get_all_players(
    league: League,
    conn: "psycopg2.connection" = None,
    file_name: str = None,
) -> None:
    """get_all_players will get the scores for all players from a given year"""
    logging.info(f"Getting all players for {league.year}")

    player_data = []

    # First fetch all player data
    league._fetch_players()

    for _, player_info in enumerate(league.espn_request.get_pro_players()):
        try:
            player_id = player_info["id"]
            player_name = player_info["fullName"]

            logging.info(f"Processing player: {player_name} ({player_id})")

            # Get detailed player info
            p = league.player_info(playerId=player_id)

            if p is None:
                logging.error(f"Player {player_id} ({player_name}) not found")
                continue

            # Process weekly stats
            for week, stats in p.stats.items():
                if week == 0:  # Skip season total stats
                    continue

                player_week_data = {
                    "player_name": p.name,
                    "player_id": p.playerId,
                    "projected_points": stats.get("projected_points", 0),
                    "actual_points": stats.get("actual_points", 0),
                    "player_position": p.position,
                    "week": week,
                    "year": league.year,
                }

                player_data.append(player_week_data)

                # Also update database if needed
                if conn is not None:
                    with conn.cursor() as cur:
                        cur.execute(
                            "INSERT INTO box_score_players (player_name, player_id, projected_points, actual_points, player_position, week, year) "
                            "SELECT %s, %s, %s, %s, %s, %s, %s "
                            "WHERE NOT EXISTS (SELECT 1 FROM box_score_players WHERE player_id = %s AND week = %s AND year = %s)",
                            (
                                p.name,
                                p.playerId,
                                stats.get("projected_points", 0),
                                stats.get("actual_points", 0),
                                p.position,
                                week,
                                league.year,
                                p.playerId,
                                week,
                                league.year,
                            ),
                        )
                        conn.commit()

            # Avoid rate limiting
            time.sleep(0.1)

        except Exception as e:
            logging.error(f"Error processing player {player_info.get('fullName', 'unknown')}: {e}")
            continue

    # Write player data to file if filename provided
    if file_name is not None and player_data:
        logging.info(f"Writing {len(player_data)} player records to {file_name}")
        with open(file_name, "w") as f:
            json.dump(player_data, f, indent=2)

    return None


def get_all_transactions(
    league: League,
    conn: "psycopg2.connection" = None,
    file_name: str = None,
) -> None:
    """get_all_transactions will get all transactions for a given league"""
    logging.info(f"Getting all transactions for {league.year}")

    all_transactions = []
    offset = 0

    if league.year < 2024:
        logging.warning("Transactions are not available for years before 2024")
        return None

    while True:
        logging.info(f"Processing transactions with offset: {offset}")
        try:
            txs = league.recent_activity(offset=offset)

            if not txs:
                break

            for tx in txs:
                tx_date = datetime.fromtimestamp(tx.date / 1000)

                for action in tx.actions:
                    team = action[0]
                    transaction_type = action[1]
                    player = action[2]
                    bid_amount = action[3]

                    transaction_data = {
                        "team_espn_id": team.team_id,
                        "player_id": player.playerId,
                        "transaction_type": transaction_type,
                        "player_name": player.name,
                        "player_position": player.position,
                        "bid_amount": bid_amount,
                        "date": tx_date.strftime("%Y-%m-%d %H:%M:%S"),
                        "year": league.year,
                    }

                    all_transactions.append(transaction_data)

                    # Update database if needed
                    if conn is not None:
                        with conn.cursor() as cur:
                            cur.execute(
                                "INSERT INTO transactions (team_id, player_id, transaction_type, date) "
                                "SELECT %s, %s, %s, %s WHERE NOT EXISTS "
                                "(SELECT 1 FROM transactions WHERE team_id = %s AND player_id = %s AND transaction_type = %s AND date = %s)",
                                (
                                    team.team_id,
                                    player.playerId,
                                    transaction_type,
                                    tx_date,
                                    team.team_id,
                                    player.playerId,
                                    transaction_type,
                                    tx_date,
                                ),
                            )
                            conn.commit()

            offset += 25
        except Exception as e:
            logging.error(f"Error processing transactions at offset {offset}: {e}")
            break

    # Write transaction data to file if filename provided
    if file_name is not None and all_transactions:
        logging.info(f"Writing {len(all_transactions)} transactions to {file_name}")
        with open(file_name, "w") as f:
            json.dump(all_transactions, f, indent=2)

    return None


def get_db_counts(conn: "psycopg2.connection") -> None:
    with conn.cursor() as cur:
        cur.execute("SELECT count(*) FROM teams")
        res = cur.fetchall()
        conn.commit()
        logging.info(f"Number of teams: {res[0][0]}")

        cur.execute("SELECT year, count(*) FROM matchups GROUP BY year ORDER BY year DESC")
        res = cur.fetchall()
        conn.commit()
        logging.info(f"Matchups: {res}")

        cur.execute("SELECT year, count(*) FROM draft_selections GROUP BY year ORDER BY year DESC")
        res = cur.fetchall()
        conn.commit()
        logging.info(f"Drafts: {res}")

        cur.execute("SELECT year, count(*) FROM box_score_players GROUP BY year ORDER BY year DESC")
        res = cur.fetchall()
        conn.commit()
        logging.info(f"Box Score Players: {res}")

        cur.execute("SELECT count(*) FROM transactions")
        res = cur.fetchall()
        conn.commit()
        logging.info(f"Transactions: {res}")

        cur.close()


def export_games_to_csv(connection_string, output_file):
    try:
        # Connect to the CockroachDB database
        conn = psycopg2.connect(connection_string)
        cursor = conn.cursor()

        # Define the query to retrieve games data
        query = """
        SELECT
            CASE WHEN m.home_team_final_score > m.away_team_final_score THEN t1.owner ELSE t2.owner END AS winner,
            GREATEST(m.home_team_final_score, m.away_team_final_score) AS winning_score,
            CASE WHEN m.home_team_final_score < m.away_team_final_score THEN t1.owner ELSE t2.owner END AS loser,
            LEAST(m.home_team_final_score, m.away_team_final_score) AS losing_score,
            m.week,
            m.year
        FROM matchups m
        JOIN teams t1 ON m.home_team_espn_id = t1.espn_id
        JOIN teams t2 ON m.away_team_espn_id = t2.espn_id
        WHERE m.home_team_final_score IS NOT NULL AND m.away_team_final_score IS NOT NULL
        """

        # Execute the query
        cursor.execute(query)

        # Fetch all rows from the executed query
        rows = cursor.fetchall()

        # Define the column headers
        headers = [desc[0] for desc in cursor.description]

        # Write the results to a CSV file
        with open(output_file, "w", newline="") as csvfile:
            csvwriter = csv.writer(csvfile)
            csvwriter.writerow(headers)  # Write the headers
            csvwriter.writerows(rows)  # Write the data rows

        logging.info(f"Data successfully exported to {output_file}")

    except Exception as e:
        logging.error(f"An error occurred: {e}")

    finally:
        # Close the cursor and connection
        cursor.close()
        conn.close()


if __name__ == "__main__":
    start = time.time()
    logging.info("Scraping fantasy football data from ESPN")

    parser = argparse.ArgumentParser()
    parser.add_argument("--year", type=int, default=2024)
    parser.add_argument("--use-database", action="store_true", help="Write data to database instead of just files")
    parser.add_argument("--output-dir", type=str, default="data", help="Directory to store output JSON files")
    args = parser.parse_args()

    # Create output directory if it doesn't exist
    if not os.path.exists(args.output_dir):
        os.makedirs(args.output_dir)

    SWID = os.environ.get("SWID")
    ESPN_S2 = os.environ.get("ESPN_S2")
    DATABASE_URL = os.environ.get("DATABASE_URL")

    if SWID is None or SWID == "":
        logging.error("SWID not set")
        exit(1)
    if ESPN_S2 is None or ESPN_S2 == "":
        logging.error("ESPN_S2 not set")
        exit(1)
    if args.use_database and (DATABASE_URL is None or DATABASE_URL == ""):
        logging.error("DATABASE_URL not set but --use-database flag is enabled")
        exit(1)

    # This was done manually but have to iterate through each year to load data
    league = League(league_id=345674, year=args.year, swid=SWID, espn_s2=ESPN_S2, debug=False)
    logging.info(f"Year: {league.year}\tCurrent Week: {league.current_week}")

    conn = None
    if args.use_database:
        conn = psycopg2.connect(DATABASE_URL)

    # Define file paths for outputs
    teams_file = os.path.join(args.output_dir, f"teams_{args.year}.json")
    matchups_file = os.path.join(args.output_dir, f"matchups_{args.year}.json")
    box_score_file = os.path.join(args.output_dir, f"box_score_players_{args.year}.json")
    draft_file = os.path.join(args.output_dir, f"draft_selections_{args.year}.json")
    transactions_file = os.path.join(args.output_dir, f"transactions_{args.year}.json")

    # Create empty files to start with
    for file_path in [teams_file, matchups_file, box_score_file, draft_file, transactions_file]:
        with open(file_path, "w") as f:
            f.write("[]")

    get_schedule(league, conn=conn, file_name=matchups_file)
    get_simple_draft(league, conn=conn, file_name=draft_file)
    get_all_transactions(league, conn=conn, file_name=transactions_file)
    # get_all_players(league, conn=conn, file_name=box_score_file)

    if args.use_database and conn is not None:
        conn.close()

    logging.info(f"Completed in {round(time.time() - start, 2)} seconds")
