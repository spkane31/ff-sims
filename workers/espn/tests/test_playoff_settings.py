from activities.schedule import extract_playoff_settings


def test_extract_playoff_settings_uses_each_rounds_scoring_weeks():
    class Settings:
        reg_season_count = 14
        playoff_team_count = 4
        matchup_periods = {**{str(week): [week] for week in range(1, 15)},
                           "15": [15], "16": [16, 17]}

    assert extract_playoff_settings(Settings()) == (4, [1, 2])
