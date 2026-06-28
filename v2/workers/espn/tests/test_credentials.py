import psycopg
import pytest
from activities.credentials import (
    AnyESPNCredentials,
    ESPNCredentials,
    get_any_espn_credentials,
    get_espn_credentials,
    get_espn_leagues,
)


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str, espn_s2: str = "s2val", swid: str = "swidval") -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, %s, %s) ON CONFLICT (espn_league_id) DO UPDATE SET espn_s2 = EXCLUDED.espn_s2, swid = EXCLUDED.swid",
            (espn_league_id, espn_s2, swid),
        )
    conn.commit()


def test_get_espn_leagues_returns_all_rows(db_conn):
    _seed_credentials(db_conn, "111")
    _seed_credentials(db_conn, "222")
    result = get_espn_leagues(2025)
    assert "111" in result
    assert "222" in result


def test_get_espn_credentials_returns_matching_row(db_conn):
    _seed_credentials(db_conn, "333", espn_s2="myS2", swid="mySWID")
    creds = get_espn_credentials("333")
    assert isinstance(creds, ESPNCredentials)
    assert creds.espn_s2 == "myS2"
    assert creds.swid == "mySWID"


def test_get_espn_credentials_raises_for_missing(db_conn):
    with pytest.raises(ValueError, match="No credentials found"):
        get_espn_credentials("nonexistent-99")


def test_get_any_espn_credentials_returns_a_row(db_conn):
    _seed_credentials(db_conn, "444", espn_s2="anyS2", swid="anySWID")
    creds = get_any_espn_credentials()
    assert isinstance(creds, AnyESPNCredentials)
    assert creds.espn_league_id is not None
    assert creds.espn_s2 is not None
    assert creds.swid is not None
