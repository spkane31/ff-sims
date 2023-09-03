from models import Roster

# Using the matchups (which contains all rosters) and the distribution of points per position, simulate the season one time.
def simulate_season(data: dict[str, any], position_stats: dict[str, tuple[float, float]]) -> None:

    return simulate_season_from_week(data, position_stats, week=0)


def simulate_season_from_week(
    matchup_data: dict[str, any], position_stats: dict[str, tuple[float, float]], week: int = 1
) -> None:
    teams = _get_teams_from_annual_data(matchup_data)
    print(teams)

    for week, scoreboard in matchup_data.items():
        for matchup in scoreboard:
            home_roster = Roster(matchup["home_lineup"])
            away_roster = Roster(matchup["away_lineup"])

            print(
                f"{matchup['home_team']}: {home_roster.projected_points()} {home_roster.simulate_projected_score(position_stats)}"
            )
            print(
                f"{matchup['away_team']}: {away_roster.projected_points()} {away_roster.simulate_projected_score(position_stats)}"
            )

            break
        break

    return


def _get_teams_from_annual_data(matchup_data: dict[str, any]) -> list[str]:
    teams = []
    for week, scoreboard in matchup_data.items():
        for matchup in scoreboard:
            teams.append(matchup["home_team"])
            teams.append(matchup["away_team"])
        return teams
