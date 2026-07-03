"""Pure transforms from Sleeper DB rows to model inputs (no DB access)."""

from datetime import datetime, timezone

from .models import Trade


def ms_to_dt(ms: int) -> datetime:
    """Unix milliseconds -> naive UTC datetime (the model's timeline)."""
    return datetime.fromtimestamp(ms / 1000.0, tz=timezone.utc).replace(tzinfo=None)


def parse_trade(
    trade_id: str,
    created_ms: int,
    adds: dict[str, int] | None,
    draft_picks: list | None,
    waiver_budget: list | None,
) -> Trade | None:
    """Build a two-sided player trade from a sleeper_transactions row.

    `adds` maps player_id -> receiving roster_id, which fully determines the
    sides. Returns None for anything the model can't value cleanly: trades
    with draft picks or FAAB attached, and trades not between exactly two
    rosters.
    """
    if draft_picks or waiver_budget or not adds:
        return None
    sides: dict[int, list[str]] = {}
    for player_id, roster_id in adds.items():
        sides.setdefault(int(roster_id), []).append(player_id)
    if len(sides) != 2:
        return None
    (_, side_a), (_, side_b) = sorted(sides.items())
    return Trade(
        trade_id=trade_id,
        ts=ms_to_dt(created_ms),
        side_a=side_a,
        side_b=side_b,
        created_ms=created_ms,
    )
