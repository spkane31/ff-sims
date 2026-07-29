"""A tiny stand-in for a psycopg connection.

Enough surface for the db/main code under test (cursor context manager,
execute/executemany, fetchall/fetchone, commit/rollback) while recording every
statement, so tests can assert which database a query went to and in what
order writes happened. Not a Postgres emulator: responses are canned per test.
"""

from __future__ import annotations

from collections.abc import Callable


class FakeCursor:
    def __init__(self, conn: "FakeConnection") -> None:
        self.conn = conn
        self._rows: list[tuple] = []

    def __enter__(self) -> "FakeCursor":
        return self

    def __exit__(self, *exc) -> bool:
        return False

    def execute(self, sql: str, params=None) -> None:
        self.conn.statements.append(sql)
        self.conn.calls.append(("execute", sql, params))
        self.conn.maybe_fail(sql)
        self._rows = self.conn.responder(sql, params)

    def executemany(self, sql: str, seq) -> None:
        rows = list(seq)
        self.conn.statements.append(sql)
        self.conn.calls.append(("executemany", sql, rows))
        self.conn.maybe_fail(sql)
        self._rows = []

    def fetchall(self) -> list[tuple]:
        return self._rows

    def fetchone(self):
        return self._rows[0] if self._rows else None


class FakeConnection:
    def __init__(
        self,
        name: str,
        responder: Callable[[str, object], list[tuple]] | None = None,
        fail_on: str | None = None,
    ) -> None:
        self.name = name
        self.responder = responder or (lambda sql, params: [])
        self.fail_on = fail_on
        self.statements: list[str] = []
        self.calls: list[tuple] = []
        self.commits = 0
        self.rollbacks = 0
        self.closed = False

    def maybe_fail(self, sql: str) -> None:
        if self.fail_on and self.fail_on in sql:
            raise RuntimeError(f"simulated failure on: {self.fail_on}")

    def cursor(self) -> FakeCursor:
        return FakeCursor(self)

    def commit(self) -> None:
        self.commits += 1
        self.calls.append(("commit", None, None))

    def rollback(self) -> None:
        self.rollbacks += 1
        self.calls.append(("rollback", None, None))

    def close(self) -> None:
        self.closed = True

    # -- assertions helpers ---------------------------------------------------

    def ran(self, needle: str) -> bool:
        return any(needle in s for s in self.statements)

    def order_of(self, needle: str) -> int:
        """Index of the first call matching `needle` (a SQL fragment, or one of
        the bare operations "commit"/"rollback"), else -1."""
        for i, (op, sql, _) in enumerate(self.calls):
            if op == needle or (sql and needle in sql):
                return i
        return -1
