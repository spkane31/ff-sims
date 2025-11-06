import argparse
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


def upsert_team(espn_id: int, owner: str, file_name: str) -> None:
    logging.info(f"Upserting team: {espn_id} - {owner} (file: {file_name})")

    with open(file_name, "a") as f:
        json.dump({"espn_id": espn_id, "owner": owner}, f)
        f.write("\n")

    return None


def upsert_teams(
    teams: List[Dict[str, any]],
    file_name: str = None,
    year: int = datetime.now().year,
) -> None:
    """Save team data to database and/or file"""
    logging.info(f"Writing teams to file: {file_name}")

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

    # Write to file if filename provided
    if file_name is not None:
        logging.info(f"Writing {len(team_data)} teams to {file_name}")
        with open(file_name, "w") as f:
            json.dump(team_data, f, indent=2)

    return None


def upsert_matchups(
    league: League,
    file_name: str = "matchups.json",
) -> None:
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
    file_name: str = "matchups.json",
) -> None:
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


def get_schedule(league: League, file_name: str) -> None:
    logging.info(f"Upserting teams for {league.year}")
    teams_file = file_name.replace("matchups", "teams") if file_name else None
    upsert_teams(
        league.teams,
        file_name=teams_file,
        year=league.year,
    )

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
                    "completed": league.current_week >= week and matchup.home_score > 0 and matchup.away_score > 0,
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


def get_simple_draft(
    league: League,
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


def update_active_players(
    league: League,
    conn: "psycopg2.connection" = None,
) -> None:
    """get_all_players will get the scores for all players from a given year"""
    logging.info(f"Getting all players for {league.year}")

    # Get all player IDs from the database
    if conn is None:
        logging.warning("conn must not be None to fetch all players")
        return

    cursor = conn.cursor()

    cursor.execute("SELECT espn_id, position FROM players WHERE status != 'inactive'")

    all_players_espn_ids = [[row[0], row[1]] for row in cursor.fetchall()]

    logging.info(f"Found {len(all_players_espn_ids)} players in the database")

    for combo in all_players_espn_ids:
        espn_id, position = combo[0], combo[1]
        p = league.player_info(playerId=espn_id)

        if p is None:
            logging.info(f"Marking player {espn_id} as inactive")
            with conn.cursor() as cur:
                cur.execute("UPDATE players SET status = 'inactive' WHERE espn_id = %s", (espn_id,))
                conn.commit()
            continue

        # Convert to dict and print
        if p.position != position:
            logging.info(f"Updating player {espn_id} position from {position} to {p.position}")
            with conn.cursor() as cur:
                cur.execute("UPDATE players SET position = %s WHERE espn_id = %s", (p.position, espn_id))
                conn.commit()

    return


def get_all_transactions(
    league: League,
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


if __name__ == "__main__":
    start = time.time()
    logging.info("Scraping fantasy football data from ESPN")

    parser = argparse.ArgumentParser()
    parser.add_argument("--year", type=int, default=2025)
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

    # This was done manually but have to iterate through each year to load data
    league = League(league_id=345674, year=args.year, swid=SWID, espn_s2=ESPN_S2, debug=False)
    logging.info(f"Year: {league.year}\tCurrent Week: {league.current_week}")

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

    with conn:
        get_schedule(league, file_name=matchups_file)
        get_simple_draft(league, file_name=draft_file)
        get_all_transactions(league, file_name=transactions_file)
        update_active_players(league, conn=conn)

    logging.info(f"Completed in {round(time.time() - start, 2)} seconds")
