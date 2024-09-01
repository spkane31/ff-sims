import os
import psycopg2


# Create tables:
def initialize() -> None:
    conn = psycopg2.connect(os.environ["COCKROACHDB_URL"])

    with conn.cursor() as cur:
        cur.execute("SELECT now()")
        res = cur.fetchall()
        conn.commit()
        print(res)

        initialize_teams_table(cur)
        initialize_matchups_table(cur)
        conn.commit()

        cur.close()

    return


def initialize_teams_table(cursor) -> None:
    create_table_query = """
        CREATE TABLE IF NOT EXISTS teams (
            owner VARCHAR(255),
            espn_id INT UNIQUE,
            team_name VARCHAR(255)
        );
        """

    cursor.execute(create_table_query)


def initialize_matchups_table(cursor) -> None:
    create_table_query = """
        CREATE TABLE IF NOT EXISTS matchups (
            week INT,
            home_team_id INT REFERENCES teams(teams.espn_id),
            away_team_id INT REFERENCES teams(teams.espn_id),
            team1_score FLOAT,
            team2_score FLOAT
        );
        """

    cursor.execute(create_table_query)
