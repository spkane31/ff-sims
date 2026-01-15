import pytest

from src.sleeper import SleeperClient

TEST_LEAGUE_ID = "1312466892979466240"


def test_sleeper_client_initialization():
    client = SleeperClient()
    assert client is not None


def test_get_league():
    client = SleeperClient()
    league = client.get_league(TEST_LEAGUE_ID)

    # Test basic fields
    assert league.league_id == TEST_LEAGUE_ID
    assert league.name == "Temporal League 2"
    assert league.sport == "nfl"
    assert league.status == "pre_draft"
    assert league.season == "2026"
    assert league.total_rosters == 12

    # Test nested metadata
    assert league.metadata is not None
    assert league.metadata.auto_continue == "on"

    # Test nested settings
    assert league.settings is not None
    assert league.settings.num_teams == 12
    assert league.settings.playoff_teams == 6
    assert league.settings.draft_rounds == 3

    # Test nested scoring settings
    assert league.scoring_settings is not None
    assert league.scoring_settings.rec == 1.0
    assert league.scoring_settings.pass_td == 4.0
    assert league.scoring_settings.rush_yd == 0.1

    # Test roster positions
    assert len(league.roster_positions) > 0
    assert "QB" in league.roster_positions
    assert "RB" in league.roster_positions