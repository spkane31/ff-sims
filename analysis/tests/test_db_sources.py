from datetime import datetime

import pytest

from src import db
from src.config import PPR_SF_10, SEASONS
from tests.fakes import FakeConnection

S2025 = SEASONS["2025"]
WINDOW = (datetime(2025, 8, 25), datetime(2026, 7, 28))

ADP_ROWS = [("p1", 3.0), ("p2", 14.5), ("k9", 200.0)]
TRADE_ROWS = [
    # (id, created_ms, adds, draft_picks, waiver_budget, league_id)
    # — Sep 3 2025 14:30 UTC
    ("t1", 1756909800000, {"p1": 1, "p2": 2}, None, None, "lgA"),
    # picks -> unvaluable
    ("t2", 1756909900000, {"p1": 1}, [{"round": 1}], None, "lgA"),
]
SCORE_ROWS = [(1, "p1", "QB", 31.5), (2, "p2", "RB", 8.0)]
PLAYER_ROWS = [
    ("p1", "QB One", "QB"),
    ("p2", "RB Two", "RB"),
    ("k9", "Kicker Nine", "K"),
]


def _archive_responder(sql, params):
    if "sleeper_draft_picks" in sql:
        return ADP_ROWS
    if "sleeper_transactions" in sql:
        return TRADE_ROWS
    return []


def _cloud_responder(players=PLAYER_ROWS, scores=SCORE_ROWS):
    def responder(sql, params):
        if "sleeper_player_week_stats" in sql:
            return scores
        if "FROM sleeper_players" in sql:
            requested = params[0] if params else []
            return [r for r in players if r[0] in requested]
        if "pg_try_advisory_xact_lock" in sql:
            return [(True,)]
        if "max(valuation_date)" in sql:
            return [(None,)]
        return []

    return responder


def _sources(cloud_responder=None, archive_fail=None, cloud_fail=None):
    return db.DataSources(
        archive=FakeConnection("archive", _archive_responder, archive_fail),
        cloud=FakeConnection("cloud", cloud_responder or _cloud_responder(), cloud_fail),
    )


# ------------------------------------------------------------- query routing --


def test_inputs_are_routed_to_the_right_database():
    sources = _sources()
    db.load_inputs(sources, PPR_SF_10, "2025", S2025, *WINDOW)

    # complete history: archive only
    assert sources.archive.ran("sleeper_draft_picks")
    assert sources.archive.ran("sleeper_transactions")
    assert not sources.archive.ran("sleeper_players")
    assert not sources.archive.ran("sleeper_player_week_stats")

    # identities + finalized scoring: cloud only
    assert sources.cloud.ran("FROM sleeper_players")
    assert sources.cloud.ran("sleeper_player_week_stats")
    assert not sources.cloud.ran("sleeper_draft_picks")


def test_input_reads_run_in_read_only_snapshots():
    sources = _sources()
    db.load_inputs(sources, PPR_SF_10, "2025", S2025, *WINDOW)
    for conn in (sources.archive, sources.cloud):
        assert conn.ran("REPEATABLE READ, READ ONLY")
        assert conn.rollbacks >= 1  # read phase ends without committing
        assert not conn.ran("INSERT INTO")
        assert not conn.ran("DELETE FROM")


def test_adp_query_filters_the_segment_server_side():
    conn = FakeConnection("archive", _archive_responder)
    db.get_adp_picks(conn, PPR_SF_10, "2025")
    sql, params = conn.calls[0][1], conn.calls[0][2]
    assert "HAVING COUNT(*) >=" in sql
    assert params[:6] == (1.0, True, 10, "redraft", "snake", "2025")


def test_trade_window_is_filtered_server_side_in_unix_ms():
    conn = FakeConnection("archive", _archive_responder)
    trades, skipped = db.get_trades(
        conn, PPR_SF_10, "2025", datetime(2025, 8, 25), datetime(2025, 8, 26)
    )
    params = conn.calls[0][2]
    assert "t.created_at_sleeper >= %s AND t.created_at_sleeper < %s" in conn.calls[0][1]
    assert params[-2:] == (1756080000000, 1756166400000)
    # the pick-laden trade is reported as a skip, not silently dropped
    assert [t.trade_id for t in trades] == ["t1"]
    assert skipped == 1


def test_trades_carry_their_league_id():
    """League-blocked evaluation needs to know which league a trade came
    from, so the query selects it and the parser keeps it."""
    conn = FakeConnection("archive", _archive_responder)
    trades, _ = db.get_trades(
        conn, PPR_SF_10, "2025", datetime(2025, 8, 25), datetime(2025, 8, 26)
    )
    assert "t.sleeper_league_id" in conn.calls[0][1]
    assert [t.league_id for t in trades] == ["lgA"]


def test_weekly_scores_are_bounded_by_the_replay_window():
    conn = FakeConnection("cloud", _cloud_responder())
    # week 1 lands Sep 8, week 2 lands Sep 15
    inside = db.get_weekly_scores(
        conn, "2025", S2025, datetime(2025, 9, 1), datetime(2025, 9, 10)
    )
    assert [s.week for s in inside] == [1]
    everything = db.get_weekly_scores(
        conn, "2025", S2025, datetime(2025, 9, 1), datetime(2025, 10, 1)
    )
    assert [s.week for s in everything] == [1, 2]


# -------------------------------------------------------- identity resolution --


def test_profiles_resolve_in_one_bulk_query_over_the_union_of_ids():
    sources = _sources()
    inputs = db.load_inputs(sources, PPR_SF_10, "2025", S2025, *WINDOW)

    player_queries = [s for s in sources.cloud.statements if "FROM sleeper_players" in s]
    assert len(player_queries) == 1
    requested = next(
        params[0] for _, sql, params in sources.cloud.calls
        if sql and "FROM sleeper_players" in sql
    )
    assert requested == ["k9", "p1", "p2"]  # ADP + trades + scores, deduped

    by_id = {a.player_id: a for a in inputs.adp}
    assert by_id["p1"].player_name == "QB One" and by_id["p1"].position == "QB"
    assert by_id["k9"].adp == 200.0


def test_missing_cloud_identity_fails_before_any_output_mutation():
    sources = _sources(cloud_responder=_cloud_responder(players=PLAYER_ROWS[:1]))
    with pytest.raises(db.MissingPlayerIdentities) as exc:
        db.load_inputs(sources, PPR_SF_10, "2025", S2025, *WINDOW)

    assert exc.value.missing == ["k9", "p2"]
    assert "sync player metadata" in str(exc.value)
    assert not sources.cloud.ran("INSERT INTO player_valuations")
    assert not sources.cloud.ran("DELETE FROM player_valuations")


def test_resolve_players_short_circuits_on_an_empty_id_list():
    conn = FakeConnection("cloud", _cloud_responder())
    assert db.resolve_players(conn, []) == {}
    assert conn.statements == []


# ---------------------------------------------------------------- locking --


def test_advisory_lock_uses_a_stable_segment_scoped_key():
    conn = FakeConnection("cloud", _cloud_responder())
    assert db.try_advisory_xact_lock(conn, "ppr-sf-10") is True
    _, sql, params = conn.calls[0]
    assert "pg_try_advisory_xact_lock" in sql
    assert params[0] == db.ADVISORY_LOCK_NAMESPACE
    assert -(2**31) <= params[1] < 2**31
    assert params[1] != db._lock_key("ppr-sf-12")  # segments don't collide


def test_the_lock_is_transaction_scoped_and_never_committed():
    """A session lock cannot be released through a transaction-pooling
    connection pool: the pooler hands the next caller a different backend, and
    an advisory lock belongs to the session that took it, so it strands. A
    committed transaction lock would release itself early instead."""
    conn = FakeConnection("cloud", _cloud_responder())
    db.try_advisory_xact_lock(conn, "ppr-sf-10")

    assert not conn.ran("pg_try_advisory_lock(")  # not the session variant
    assert conn.commits == 0  # committing here would drop the lock
    assert not conn.ran("pg_advisory_unlock")  # nothing to leak


def test_contended_lock_reports_failure_rather_than_waiting():
    conn = FakeConnection(
        "cloud", lambda sql, params: [(False,)] if "advisory" in sql else []
    )
    assert db.try_advisory_xact_lock(conn, "ppr-sf-10") is False
    assert not conn.ran("pg_advisory_xact_lock(")  # never the blocking variant


def test_missing_database_url_fails_before_connecting(monkeypatch):
    monkeypatch.setenv(db.CLOUD_URL_ENV, "postgres://cloud/db")
    monkeypatch.delenv(db.ARCHIVE_URL_ENV, raising=False)
    monkeypatch.setattr(
        db.psycopg, "connect", lambda *a, **k: pytest.fail("must not connect")
    )
    with pytest.raises(RuntimeError, match=db.ARCHIVE_URL_ENV):
        with db.open_sources():
            pass


# ------------------------------------------- unvaluable positions --


IDP_TRADE_ROWS = [
    ("t1", 1756909800000, {"p1": 1, "p2": 2}, None, None, "lgA"),  # both fantasy
    ("t3", 1756909900000, {"p1": 1, "lb1": 2}, None, None, "lgA"),  # one IDP
]
IDP_PLAYER_ROWS = PLAYER_ROWS + [("lb1", "Line Backer", "LB")]


def test_a_trade_touching_a_position_scores_never_cover_is_dropped():
    """Weekly scores only ever return fantasy positions, so a belief created
    for an IDP could never be corrected — it would hold whatever the trade
    stream implied for the rest of the run."""
    sources = db.DataSources(
        archive=FakeConnection(
            "archive",
            lambda sql, params: (
                ADP_ROWS if "sleeper_draft_picks" in sql
                else IDP_TRADE_ROWS if "sleeper_transactions" in sql
                else []
            ),
        ),
        cloud=FakeConnection("cloud", _cloud_responder(players=IDP_PLAYER_ROWS)),
    )
    inputs = db.load_inputs(sources, PPR_SF_10, "2025", S2025, *WINDOW)

    assert [t.trade_id for t in inputs.trades] == ["t1"]
    assert inputs.skipped_nonfantasy == 1
    assert inputs.skipped_trades == 0  # a different reason, counted separately
    # the whole trade goes: p1 keeps only the trade it shared with a valuable
    # player, rather than half of the dropped one
    assert "lb1" not in inputs.players
