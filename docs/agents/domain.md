# Domain Docs

This is a single-context repository. Engineering skills should use the following sources to understand its domain language and architectural decisions.

## Before exploring, read these when present

- `CONTEXT.md` at the repository root
- Relevant ADRs under `docs/adr/`
- Relevant design history under `docs/superpowers/specs/` and `docs/superpowers/plans/`

If a source does not exist, proceed silently. Domain glossaries and ADRs are created lazily when the project resolves terminology or architectural decisions that merit them.

## Use the glossary's vocabulary

When output names a domain concept, use the term defined in `CONTEXT.md`. Do not drift to synonyms the glossary explicitly avoids.

If a needed concept is absent, reconsider whether the language belongs to the project or note the gap for a future documentation session.

## Flag ADR conflicts

If proposed work contradicts an existing ADR, surface that conflict explicitly rather than silently overriding the decision.
