import argparse
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


def upsert_team(conn: "psycopg2.connection", espn_id: int, owner: str) -> None:
    with conn.cursor() as cur:
        cur.execute("SELECT * FROM teams WHERE espn_id = %s", (espn_id,))
        if cur.fetchone():
            cur.execute("UPDATE teams SET owner = %s WHERE espn_id = %s", (owner, espn_id))
        else:
            cur.execute("INSERT INTO teams (owner, espn_id) VALUES (%s, %s)", (owner, espn_id))
        conn.commit()

    return None


def upsert_matchup(
    conn: "psycopg2.connection",
    week: int,
    year: int,
    home_team: int,
    away_team: int,
    home_team_score: float,
    away_team_score: float,
    home_team_projected_score: float,
    away_team_projected_score: float,
    completed: bool,
) -> None:
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


def get_schedule(league: League, conn: "psycopg2.connection") -> None:
    for team in league.teams:
        upsert_team(conn, team.team_id, " ".join([team.owners[0]["firstName"], team.owners[0]["lastName"]]))

    print(f"Creating matchups based on {league.year}")
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

    for week in range(1, 15):
        print(f"Year: {league.year}\tWeek: {week}")
        if week > league.current_week and datetime.now().year == league.year:
            break
        if league.year < 2019:
            for matchup in league.scoreboard(week=week):
                if not hasattr(matchup, "away_team") or not hasattr(matchup, "home_team"):
                    break
                upsert_matchup(
                    conn,
                    week,
                    league.year,
                    matchup.home_team.team_id,
                    matchup.away_team.team_id,
                    matchup.home_score,
                    matchup.away_score,
                    -1,
                    -1,
                    True,
                )
        else:
            # box_scores func only works for the current year
            for matchup in league.box_scores(week=week):
                if matchup.away_team == 0 or matchup.home_team == 0:
                    break
                upsert_matchup(
                    conn,
                    week,
                    league.year,
                    matchup.home_team.team_id,
                    matchup.away_team.team_id,
                    matchup.home_score,
                    matchup.away_score,
                    matchup.home_projected,
                    matchup.away_projected,
                    league.current_week > week,
                )

                home_team_id = matchup.home_team.team_id
                away_team_id = matchup.away_team.team_id

                if league.year == datetime.now().year and week < league.current_week:
                    for player in matchup.home_lineup:
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

    return None


def write_box_score_players_to_db(
    box_score_players: List[Dict[str, any]], year: int, conn: "psycopg2.connection"
) -> None:
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

    return None


def get_simple_draft(league: League, conn: "psycopg2.connection") -> None:
    with conn.cursor() as cur:
        for pick in league.draft:
            player_info = league.player_info(playerId=pick.playerId)
            # If the selection exists, add the position (which does not exist as of 2024.11.03)
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
            if res[0] == 1:
                cur.execute(
                    "UPDATE draft_selections SET player_position = %s WHERE player_name = %s AND owner_espn_id = %s AND round = %s AND pick = %s AND year = %s",
                    (
                        player_info.position,
                        pick.playerName,
                        pick.team.team_id,
                        pick.round_num,
                        pick.round_pick,
                        league.year,
                    ),
                )

            cur.execute(
                "INSERT INTO draft_selections (player_name, player_id, player_position, owner_espn_id, round, pick, year) SELECT %s, %s, %s, %s, %s, %s, %s WHERE NOT EXISTS (SELECT 1 FROM draft_selections WHERE player_name = %s AND owner_espn_id = %s AND round = %s AND pick = %s AND year = %s)",
                (
                    pick.playerName,
                    pick.playerId,
                    player_info.position,
                    pick.team.team_id,
                    pick.round_num,
                    pick.round_pick,
                    league.year,
                    pick.playerName,
                    pick.team.team_id,
                    pick.round_num,
                    pick.round_pick,
                    league.year,
                ),
            )

        conn.commit()
        cur.close()

    return None


def get_all_players(league: League, conn: "psycopg2.connection") -> None:
    """get_all_players will get the scores for all players from a given year"""

    # Query all players
    with conn.cursor() as cur:
        cur.execute("SELECT player_id FROM box_score_players WHERE year = %s", (league.year,))
        all_players = cur.fetchall()
        logging.info(f"Number of players: {len(all_players)}")

        for player in all_players:
            try:
                player_info = league.player_info(playerId=player[0])

                logging.info(f"Player: {player_info.name}")

                cur.execute(
                    "SELECT week FROM box_score_players WHERE player_id = %s AND year = %s",
                    (player[0], league.year),
                )
                weeks = cur.fetchall()
                set_of_weeks = set([week[0] for week in weeks])

                for week, stats in player_info.stats.items():
                    if week not in set_of_weeks and week != 0:
                        cur.execute(
                            "INSERT INTO box_score_players (player_name, player_id, projected_points, actual_points, player_position, week, year) SELECT %s, %s, %s, %s, %s, %s, %s WHERE NOT EXISTS (SELECT 1 FROM box_score_players WHERE player_id = %s AND week = %s AND year = %s)",
                            (
                                player_info.name,
                                player_info.playerId,
                                stats.get("projected_points", 0),
                                stats.get("actual_points", 0),
                                player_info.position,
                                week,
                                league.year,
                                player_info.playerId,
                                week,
                                league.year,
                            ),
                        )
                        conn.commit()
                        print(f"\tinserted week {week}")

                        break
            except Exception as e:
                print(f"\tError: {e}")
                continue

    return None


def get_all_transactions(league: League, conn: "psycopg2.connection") -> None:
    offset = 0
    with conn.cursor() as cur:
        while True:
            logging.info(f"Offset: {offset}")
            txs = league.recent_activity(offset=offset)
            offset += 25
            if len(txs) == 0:
                conn.commit()
                return None
            logging.info(f"Number of transactions: {len(txs)}")
            for tx in txs:
                tx_date = datetime.fromtimestamp(tx.date / 1000)
                for action in tx.actions:
                    team = action[0]
                    transaction_type = action[1]
                    player = action[2]
                    _bid_amount = action[3]  # My league does not use bids so this is always 0
                    logging.info(f"Team: {team.team_id}\tPlayer: {player.name}\tType: {transaction_type}")

                    cur.execute(
                        "INSERT INTO transactions (team_id, player_id, transaction_type, date) SELECT %s, %s, %s, %s WHERE NOT EXISTS (SELECT 1 FROM transactions WHERE team_id = %s AND player_id = %s AND transaction_type = %s AND date = %s)",
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


if __name__ == "__main__":
    start = time.time()
    logging.info("Scraping fantasy football data from ESPN")

    parser = argparse.ArgumentParser()
    parser.add_argument("--year", type=int, default=2024)
    args = parser.parse_args()

    SWID = os.environ.get("SWID")
    ESPN_S2 = os.environ.get("ESPN_S2")
    DATABASE_URL = os.environ.get("DATABASE_URL")

    if SWID is None or SWID == "":
        logging.error("SWID not set")
        exit(1)
    if ESPN_S2 is None or ESPN_S2 == "":
        logging.error("ESPN_S2 not set")
        exit(1)
    if DATABASE_URL is None or DATABASE_URL == "":
        logging.error("DATABASE_URL not set")
        exit(1)

    # This was done manually but have to iterate through each year to load data
    league = League(league_id=345674, year=args.year, swid=SWID, espn_s2=ESPN_S2, debug=False)
    logging.info(f"Year: {league.year}\tCurrent Week: {league.current_week}")

    conn = psycopg2.connect(DATABASE_URL)

    get_db_counts(conn)

    get_schedule(league, conn)
    get_simple_draft(league, conn)
    get_all_players(league, conn)
    get_all_transactions(league, conn)

    print(f"Completed in {round(time.time() - start, 2)} seconds")
