# Player Valuation Model — Architecture

**Status:** Draft
**Date:** 2026-06-27
**Author:** Sean Kane

---

## Overview

This document describes the architecture for a player valuation model that produces a single numeric value per player, updated daily, usable across all three fantasy league formats (redraft, keeper, dynasty). The primary use case is a **trade calculator**: summing values on each side of a proposed trade to determine fairness, and surfacing trade suggestions between two teams of roughly equal total value.

The model combines three distinct data signals — draft position, trade history, and on-field performance — whose relative importance shifts naturally across the NFL calendar.

---

## Goals

- Produce a daily valuation for every rostered Sleeper player across all 18 model segments
- Values must be on a scale where multi-player trades are mathematically coherent (a #1 player can exceed the sum of a #3 + #12)
- Trade calculator: sum values on each trade side and compare
- Trade suggester: find pairs of players across two teams whose values are roughly equal
- Support all three league formats with format-appropriate signals and adjustments

---

## Model Segmentation

The model runs as **18 independent pipelines**, one per combination of:

| Dimension | Values |
|---|---|
| League format | redraft / keeper / dynasty |
| Scoring format | PPR / 0.5PPR / 0PPR |
| QB configuration | standard (1 QB) / superflex (2+ QB) |

Each pipeline trains on trades and drafts from leagues matching that segment. League size, exact TE premium coefficient, and flex slot configuration are **not** segmentation dimensions — they are used at query time to compute the VORP replacement level for a specific league.

This keeps each segment's training data dense while still personalizing output to a league's exact settings.

---

## The Three Signals

### 1. Draft Signal

**Source:** `sleeper_draft_picks`, filtered to completed drafts in the matching segment.

**Computation:** Average Draft Position (ADP) across all qualifying drafts. Earlier picks map to higher raw scores. The draft signal anchors the model pre-season and serves as the regularization prior for the trade signal solver (see below). It also stabilizes valuations for players who are rarely traded.

**Lifecycle:** Recomputed daily. Stable once a season's drafts complete; grows more representative as more leagues draft.

### 2. Trade Signal

**Source:** `sleeper_transactions` where `type = 'trade'`, filtered to the matching segment.

**Computation:** Additive least squares optimization. Each trade contributes the constraint:

```
Σ value(players on side A)  ≈  Σ value(players on side B)
```

The solver minimizes total squared trade imbalance across all trades:

```
minimize  Σ_trades [ Σ value(side_A) - Σ value(side_B) ]²
          + λ × Σ_players [ value(player) - aDP_prior(player) ]²
```

The regularization term (weighted by λ) anchors player values toward their ADP prior. This serves two purposes: it prevents the underdetermined system from drifting when trade data is sparse, and it explicitly models the trade signal as **deviation from draft consensus** rather than an independent estimate. Early-season trades largely reflect ADP anyway — managers haven't seen enough performance to deviate much — so treating the trade signal as independent of the draft signal would double-count the same information. The regularization makes this relationship explicit: as trade volume grows and the market forms its own view, values pull away from ADP naturally.

Draft picks included in trades are assigned ADP-derived prior values and participate in the same optimization.

**Phase 1 note:** All trades within a segment are weighted equally. A planned Phase 2 improvement is to weight trades by league activity (`√(num_transactions_in_league)`), which down-weights inactive leagues where trades may reflect desperation or collusion rather than market consensus.

**Lifecycle:** Full recompute each daily run (warm-started from previous day's values). Adding new trades can shift all scores globally, so incremental updates are not used.

**Data:** 80k trades across 98k leagues at time of writing. The additive least squares approach handles multi-player trades (2-for-1, 3-for-2) correctly without losing information by forcing them into pairwise comparisons.

### 3. Performance Signal

**Source:** Sleeper unofficial weekly stats endpoint: `GET https://api.sleeper.app/v1/stats/nfl/regular/{season}/{week}`

Returns per-player weekly stats despite not appearing in the official Sleeper API docs at [docs.sleeper.com](https://docs.sleeper.com). This endpoint is widely used by the Sleeper developer community and is considered stable in practice.

**Fallback chain if the endpoint is removed or changed:**
1. Aggregate player scores from `GET /league/{league_id}/matchups/{week}` across all fetched leagues (data already collected by the transaction sync)
2. ESPN `box_scores.actual_points` from the existing ETL (covers ESPN leagues only; may have coverage gaps)
3. Third-party stats API (e.g., SportsDataIO) — adds an external paid dependency, last resort

**Computation:** Points Above Replacement (PAR) per player per week, with Bayesian shrinkage toward the preseason prior.

```
PAR(player, week) = actual_points(player, week) - replacement_level(position, week)
```

**Replacement level** is computed dynamically each week from actual performance rankings rather than a fixed baseline. For a given position, the replacement player is the `(N+1)`th-ranked player by points scored that week, where `N = num_active_leagues_in_segment × avg_starters_per_position`. This naturally tracks years where positional depth is thin or deep.

**Bayesian shrinkage** prevents overreaction to small samples. Raw PAR is shrunk toward the preseason prior (derived from the draft signal) based on games observed:

```
performance_score(player) = (n_obs × mean_PAR + n_prior × preseason_rating)
                            / (n_obs + n_prior)
```

`n_prior` controls how many observed games are needed before raw PAR dominates the preseason prior. Phase 1 starting value: `n_prior = 4` (four games of observed data carry equal weight to the prior). This means a player with 1 elite week is still largely anchored to their preseason rating, while a player with 8+ weeks of data is mostly driven by observed performance.

Weekly PAR scores are aggregated with **recency decay**: more recent weeks weighted more heavily. Decay function: `weight(week) = λ^(current_week - week)` with `λ ≈ 0.85`.

**Lifecycle:** Updated meaningfully only after each NFL game week completes, but the pipeline runs daily to incorporate retroactive stat corrections.

**Note:** Fetching Sleeper weekly stats for all players is a new data dependency. This will be tracked as a separate GitHub issue.

---

## Sparse Player Fallback

Players with missing signals (rookies before their first game, UDFA pickups, practice squad promotions) need a defined fallback hierarchy rather than a zero or null value:

1. **All three signals available** → standard computation
2. **No trade history** → draft + performance signals only, trade weight redistributed proportionally to the other two
3. **No draft ADP** (undrafted free agents, in-season call-ups) → performance signal + positional average as the prior in the Bayesian shrinkage formula
4. **No performance data** (injured all season, never activated) → draft signal only; if also no ADP, use the positional replacement-level value as a floor
5. **No data at all** (newly signed, no history) → floor value equal to the positional replacement level for that segment

Players at replacement level are explicitly non-zero — roster spots have intrinsic value (see VORP section).

---

## Signal Normalization

Before blending, each signal is converted to **percentile rank within the segment** (0–1 across all players with a valid score for that signal), then scaled to a common range. Rank normalization is used rather than z-score because:

- It is robust to outliers — one historically elite player does not compress the distribution for everyone else
- It handles the very different natural scales of the three signals (ADP pick numbers, least squares solver outputs, and PAR point totals) without assuming Gaussian distributions

Players at the sparse fallback floor receive a percentile of 0 for the missing signal before redistribution.

---

## Time-Decayed Blend

Normalized signals are combined with weights that shift smoothly across the NFL calendar:

| Phase | NFL Week | Draft | Trade | Performance |
|---|---|---|---|---|
| Pre-season | < 1 | 0.85 | 0.10 | 0.05 |
| Early season | 1–5 | 0.40 | 0.35 | 0.25 |
| Mid-season | 6–10 | 0.15 | 0.35 | 0.50 |
| Late season | 11–17 | 0.05 | 0.20 | 0.75 |

Weights between phases are **linearly interpolated by NFL week** — no hard jumps. The draft signal retains a small non-zero weight throughout because the least squares solver already incorporates it as a regularization prior; the blending weight here reflects residual anchoring for players with almost no trade or performance data.

### Blend Weights as Configurable Parameters

Phase 1 uses the heuristic weights above, stored as configurable constants per segment (not hardcoded). This enables a future **weight optimization phase** described in Future Improvements below.

A known limitation of calendar-based weights: they do not respond to information shocks (e.g., a week-3 ACL tear should immediately collapse the draft weight for that player regardless of what week it is). Injury adjustments handle the most severe cases; adaptive information-content-based weights are a Phase 2 improvement.

---

## Dynasty & Keeper Age Curve

After blending, **dynasty and keeper formats** apply a position-specific age curve multiplier. This is not applied to redraft.

| Position | Initial Peak Age Range (Phase 1 prior) |
|---|---|
| RB | 22–25 |
| WR | 23–27 |
| QB | 24–32 |
| TE | 23–28 |

These ranges are **Phase 1 starting-point priors**, not permanent constants. The curves will be learned from historical dynasty startup draft data: dynasty startup ADPs aggregate what managers collectively believe about player age trajectories, and fitting a curve to ADP-by-age across positions yields empirical peak estimates. As dynasty startup draft data accumulates in `sleeper_draft_picks`, the curve parameters will be refitted. Hand-tuned priors are used only until enough startup data is available to fit reliably.

Players below peak age receive a future-value premium; players past peak receive a discount. The multiplier is derived from `sleeper_players.age` and `years_exp`.

---

## VORP Adjustment (Query Time)

The blended score (including age curve for dynasty/keeper) is stored in `player_valuations` as the **raw segment score**. When a query arrives for a specific league (e.g., trade calculator), the replacement level is computed from that league's actual settings:

```
replacement_rank(position) = num_teams × starters_at_position + 1
```

Flex slots contribute fractional replacement-level pressure based on historical positional usage rates in that segment. The replacement player's raw segment score is subtracted from each player's score:

```
vorp_score(player) = raw_segment_score(player) - raw_segment_score(replacement_player_at_position)
```

Players below replacement level floor at a small positive value — not zero — because roster spots have intrinsic value equal to approximately the best freely available player. This is what makes 2-for-1 trades sensible: the team receiving two players must drop someone to waivers, and that dropped player's value (≈ replacement level) is the hidden cost of accepting the extra roster slot.

---

## Exponential Value Curve

The VORP-adjusted score is passed through an exponential curve to produce the final user-facing value:

```
value = A × exp(k × vorp_score)
```

Where `A` and `k` are tunable constants controlling scale and steepness. The target calibration is approximately:

- Top player: ~10,000
- Solid starter (e.g., WR2): ~3,000–5,000
- Fringe starter / flex: ~1,000–2,000
- High-end backup: ~300–800

This scale makes multi-player trades mathematically coherent. Trading a 10,000-value player for a 5,000 + 3,000 player (8,000 total) shows a clear deficit. The exponential gap at the top reflects the real scarcity of elite fantasy producers.

Both `A` and `k` are configurable per segment. Phase 1 calibration: tune empirically once first-season valuations are generated, targeting alignment with publicly available consensus values (FantasyCalc, KTC) as a sanity check. The curve itself is for UX interpretability; it is not derived from the optimization and should not be calibrated against the same trade data used to train the solver (circularity).

---

## Output Schema

Values are written to the existing `player_valuations` table, extended to support segments and per-signal scores:

```sql
player_valuations (
    sleeper_player_id   TEXT,
    valuation_date      DATE,
    league_format       TEXT,   -- redraft / keeper / dynasty
    scoring_format      TEXT,   -- ppr / half_ppr / standard
    qb_config           TEXT,   -- standard / superflex
    draft_signal        FLOAT,  -- percentile-ranked ADP score
    trade_signal        FLOAT,  -- least squares solver output (deviation from ADP prior)
    perf_signal         FLOAT,  -- recency-decayed, shrinkage-adjusted PAR score
    blended_score       FLOAT,  -- time-weighted blend of above
    age_curve_factor    FLOAT,  -- multiplier (1.0 for redraft)
    raw_segment_score   FLOAT,  -- blended_score × age_curve_factor
    PRIMARY KEY (sleeper_player_id, valuation_date, league_format, scoring_format, qb_config)
)
```

The VORP subtraction and exponential curve are applied at query time. Storing the raw segment score keeps the pipeline output independent of any specific league's settings.

---

## Data Pipeline

The valuation pipeline runs as a **daily Temporal workflow**:

1. **Draft signal recompute** — recalculate ADP from all completed drafts per segment
2. **Trade signal recompute** — re-run additive least squares solver per segment, warm-started from previous day's values, regularized toward current ADP prior
3. **Performance signal recompute** — pull latest weekly PAR from Sleeper stats endpoint, apply dynamic replacement level, Bayesian shrinkage, and recency decay
4. **Normalize** — convert each signal to percentile rank within the segment
5. **Blend** — apply time-decayed weights for the current NFL week across all 18 segments
6. **Age curve** — apply position-specific multiplier for keeper and dynasty segments
7. **Write** — upsert `player_valuations` with per-signal scores and raw segment score

---

## Trade Calculator Query

At query time for a specific league:

1. Look up `player_valuations` for the matching `(league_format, scoring_format, qb_config)` segment and most recent `valuation_date`
2. Compute `replacement_rank` per position from the league's `(num_teams, roster_positions)`
3. Compute `vorp_score = raw_segment_score - replacement_score_at_position`
4. Apply exponential curve: `value = A × exp(k × vorp_score)`
5. Sum values for each trade side and compare

---

## Validation Metrics

Success of each model version is measured against explicit metrics before any version is considered production-ready:

| Metric | Description |
|---|---|
| **Held-out trade error** | Hold out 20% of trades; measure mean squared error of `\|Σ value(side_A) - Σ value(side_B)\|` across held-out set. Primary training objective. |
| **Spearman vs FantasyCalc (redraft)** | Rank correlation of player values against FantasyCalc consensus for current-season redraft |
| **Spearman vs KTC (dynasty/keeper)** | Rank correlation against KeepTradeCut dynasty values |
| **Next-month ADP shift** | Does a rising trade value predict a rising ADP in the following month? |
| **End-of-season points** | Do mid-season values (week 8) predict end-of-season total fantasy points? |

The held-out trade error is the ground truth metric for the solver. External comparisons (FantasyCalc, KTC) are sanity checks, not optimization targets — the model may legitimately diverge from consensus when data supports it.

---

## Phase 2: Linear-Gaussian State-Space Model

Phase 1 is a defensible MVP. Its hand-tuned constants (blend weights, recency decay, shrinkage prior, ADP regularization) are all approximations of the same underlying coherent model: a **linear-Gaussian state-space model** solved with a Kalman filter and RTS smoother. Phase 2 replaces the heuristics with that model, which makes the parameters learnable from data rather than tuned by hand.

### The latent state

Each player `i` has a single hidden value `x[i,t]` at time `t` — their true trade-fair value in solver units (not percentiles). Everything observed — draft position, trades, on-field performance — is a noisy window onto this hidden state.

### Transition: the random walk

Between time steps, value drifts:

```
x[i,t] = x[i,t-1] + w[i,t]     w ~ N(0, q_i · Δt)
```

`q_i` is the **process noise** — how fast a player's value is allowed to move. Small `q` means sticky (veteran QB); large `q` means volatile (injury-prone RB). Process noise is set per position and can be spiked at a known shock (confirmed season-ending injury) to let the posterior snap immediately — no calendar logic required.

**Sparse player entry:** players do not enter at `t=0`. A rookie, UDFA, or mid-season call-up enters at their arrival time with an initial prior derived from their position and, where available, draft slot. This replaces the five-rung sparse-player fallback ladder with a single mechanism.

**ADP prior:** the initial state `x[i,0] ~ N(adp_prior_i, P_0)` where `P_0` encodes confidence in ADP. High `P_0⁻¹` (tight prior) means ADP dominates early; as trades and performance accumulate they swamp it. This is the Phase 1 ADP regularization term `λ`, now as a precision parameter estimated from data.

### Observation 1: trades

A trade linking side A and side B contributes the constraint:

```
Σ x[i∈A,t] − Σ x[j∈B,t] − ρ·(|B|−|A|) = ε     ε ~ N(0, r_trade)
```

The observation is 0 (trades are assumed fair); the noise `ε` is how far from fair any given trade actually was. `ρ` is the roster-spot value — the cost of accepting an extra player — now a free parameter estimated from unbalanced trades rather than derived from replacement level calculations. In matrix form, the observation row is the ±1 incidence vector — **exactly the row the Phase 1 additive least squares solver already builds**. The Phase 1 solver is the single-timestep MAP estimate of this model.

### Observation 2: performance

PAR is in points, not value units. A measurement link converts between them:

```
PAR[i,t] = c0 + c1·x[i,t] + η     η ~ N(0, s_perf)
```

`c1` is the loading that maps value-space to points-space, **estimated from data**. This single equation replaces the entire Phase 1 normalization-then-blend pipeline for the performance signal. Each signal stays in its own units; the model learns the conversion rather than forcing everything through percentile ranks.

### How Phase 1 components map to Phase 2

| Phase 1 component | Phase 2 equivalent |
|---|---|
| Calendar blend weights (0.85 / 0.40 / …) | Kalman precision ratios — emergent, not tuned |
| Recency decay λ ≈ 0.85 | Process noise `q` — principled per-position forgetting |
| Bayesian shrinkage n_prior = 4 | Prior-to-observation precision ratio |
| ADP regularization λ in solver | Initial state prior precision `P_0⁻¹` |
| VORP roster-spot value | Free parameter `ρ` estimated from unbalanced trades |
| Sparse player fallback ladder | Player enters at arrival time with position prior |
| Injury adjustment (manual discount) | Spike `q` at injury event; value coasts with growing uncertainty |
| Adaptive blend weights (future item) | Filter computes information-weighted update automatically |
| Learned blend weights (future item) | EM M-step fits precision ratios from data |
| Market momentum signals (future item) | Absorbed — the random walk tracks momentum implicitly |

### Fitting: filter, smoother, EM

**States given hyperparameters** — run the Kalman filter forward (online estimates) and the RTS smoother backward (retrospective estimates). The result is an exact Gaussian posterior over every player's value at every time step.

Because trades couple players (Stafford and Jefferson in the same trade have correlated posteriors), the joint covariance is a large sparse object. The right representation is the **information (precision) form**: track `Λ = P⁻¹` rather than `P`. Each trade adds a sparse rank-2 update to `Λ` touching only the players involved. Across a season, `Λ` has the sparsity structure of the trade graph (a weighted Laplacian) plus a diagonal from performance and prior. Sparse Cholesky on this is fast — the same sparse normal-equations math as the Phase 1 solver.

**Hyperparameters** (`q`, `r_trade`, `s_perf`, `c1`, `ρ`, `P_0`) — because the model is linear-Gaussian, the states can be marginalized out analytically. The Kalman filter produces the exact marginal likelihood of the hyperparameters via the prediction-error decomposition, so hyperparameter fitting never requires sampling over latent states.

Fitting proceeds via **EM**:
- **E-step:** run the Kalman filter + RTS smoother to get posterior state estimates and covariances (closed form)
- **M-step:** update each variance parameter in closed form given the smoothed statistics

This is the Phase 1 "learned blend weights" future item, done properly: the fitted precision ratios are the weights. An alternative to EM is HMC on the marginal likelihood (NumPyro or Stan) for full posteriors over hyperparameters — low-dimensional parameter space, feasible since states are marginalized out.

### Tooling

Each of the 18 segments is independent and maps directly to the existing per-segment Temporal workflow structure. A season is ~120 daily (or ~18 weekly) time steps over ~1–2k active players per segment — minutes per segment with sparse linear algebra.

Candidate libraries:
- **Dynamax** (JAX) — built for linear-Gaussian SSMs with parameter learning via EM; best fit for this use case
- **Hand-rolled sparse information filter** (SciPy) — full control over trade-incidence sparsity; more work but no JAX dependency
- `statsmodels` state space — too rigid for the custom trade-incidence observation matrix; avoid

### Identifiability

The zero-point identifiability issue from Phase 1 carries over unchanged. `n`-for-`n` trades are invariant to a global additive constant, so the absolute value level is pinned only by the ADP prior, performance observations, and `ρ` via unbalanced trades. The ADP prior remains load-bearing for scale calibration.

### Honest build costs

- **Debugging a smoother is harder than a least-squares solve.** Covariances can go non-PSD from numerical drift; failures are often silent. Budget time for numerical validation.
- **`c1` is load-bearing.** The performance measurement link replaces normalization. If it is misspecified, fusion is quietly wrong. Validate it in isolation against held-out performance weeks before connecting it to the full filter.
- **Gaussian trade noise is outlier-sensitive.** Collusion and dump trades are heavy-tailed. A Student-t observation model fixes this but breaks closed-form Kalman, pushing toward a robust filter variant or MCMC. The Phase 1 trade-weighting-by-league-activity heuristic is the manual version of the same robustness need; retain it as a pre-filter step even in Phase 2.

---

## Future Improvements

Most of the items originally in this section are absorbed by the Phase 2 state-space model (see mapping table above). The following remain out of scope for both Phase 1 and Phase 2:

- **Roster-context trade calculator** — marginal player value accounting for a team's current roster composition (a 4th QB adds near-zero value to a team already stacked at QB); requires knowing each team's live roster at query time
- **Age curve refinement** — refit position-specific peaks from dynasty startup ADP data as it accumulates; applies to both Phase 1 and Phase 2 (the process noise `q` in Phase 2 implicitly captures aging volatility but an explicit age prior still improves cold-start)

---

## Open Issues

- **GitHub issue:** New Temporal activity to fetch Sleeper weekly player stats (`GET /stats/nfl/regular/{season}/{week}`) on a weekly schedule, with fallback chain documented above
- **GitHub issue:** Injury adjustment — when `sleeper_players.status` indicates a significant injury (IR, PUP, season-ending), apply a discount factor to the player's raw segment score. Needs to distinguish between "out this week" (small discount) and "season-ending ACL" (near-zero value). Source: `sleeper_players.status` and `injury_status` fields.
- **Schema migration:** Extend `player_valuations` to add segment columns and per-signal score columns (replaces current single-value schema)
- **Least squares solver selection:** Choose between closed-form pseudo-inverse (fast, exact, memory-intensive at scale) and gradient descent (scalable, tunable convergence); benchmark at 80k+ trade scale
- **Age curve fitting:** Implement curve-fitting pass over dynasty startup draft data once sufficient seasons are available; until then, use Phase 1 priors
- **Blend weight calibration:** Seasonal calibration pass using trade prediction holdout (Phase 2)
- **`k` and `A` calibration:** Exponential curve constants need empirical tuning per segment once first-season valuations are generated
