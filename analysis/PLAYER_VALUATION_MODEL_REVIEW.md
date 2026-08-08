# Player Valuation Model Review

**Date:** 2026-07-31  
**Status:** Complete  
**Scope:** The Python model under `analysis/`, the supplied `ppr-sf-10` 2025 replay output, the model architecture documents, deterministic diagnostic probes, and the published methodology of a public trade-based consensus site.

## Executive conclusion

The low values are not primarily a bad `lambda`, `rho`, or variance setting. The current trade observation has a structural flat-value optimum, and the replay feeds it enough trade updates to overwhelm the ADP curve and weekly scoring evidence. The reported top value of 4,682 is almost exactly the configured seed curve's value at rank 20, even though the seed's rank-1 value is 10,000.

The implementation also differs materially from the intended architecture:

- The design keeps draft, trade, and performance as separately normalized signals, then applies VORP and an exponential presentation curve at query time.
- The implementation seeds one raw value from an exponential ADP curve, mutates that same value directly with every trade and score event, and publishes the mutated value without a final calibration step.

This is why the exponential curve is visible at initialization but not in the final output. More tuning inside the present update rule will move the collapse point; it will not make the model identifiable or reliably recover the intended curve.

The recommended direction is to retain the input, staging, replay, and snapshot infrastructure, but rebuild the estimator around four separate concepts:

1. **Market value:** robust, time-weighted inference from actual trades, anchored so the scale and shape are identifiable.
2. **Expected football value:** rest-of-season projected points above position-specific replacement, not a backward-looking market price.
3. **League-specific replacement and waiver cost:** computed from the queried league's roster and free agents, not fitted as one global constant from trades.
4. **Team utility:** post-trade lineup points or win probability, which is required to claim that one or both teams improve.

## Evidence and limits

The repository's complete 107-test baseline passes. The exact staged bundle named in the supplied run was created on `rosebud` and is not present in this worktree, so I could not rerun those exact 39,214 trades or inspect their league-level distribution. The supplied output is nevertheless sufficient to establish the production symptom, and deterministic probes against the actual model code reproduce the causal mechanisms below.

The current branch contains additional outlier diagnostics added after the shorter diagnostic block shown in the supplied run. An exact empirical outlier-share analysis should be repeated after copying the staged bundle into this worktree.

## What the current model actually does

The architecture calls for separately computed signals, percentile normalization, a time-varying blend, query-time positional VORP, and a final exponential curve ([modeling.md:43-78](/Users/seankane/github.com/ff-sims-run-model-daily/docs/arch/modeling.md:43), [modeling.md:130-158](/Users/seankane/github.com/ff-sims-run-model-daily/docs/arch/modeling.md:130), [modeling.md:179-214](/Users/seankane/github.com/ff-sims-run-model-daily/docs/arch/modeling.md:179)).

The code instead does this:

1. Map ADP directly to `10,000 * exp(-0.04 * (rank - 1))` once ([valuation.py:38-47](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:38), [valuation.py:215-227](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:215)).
2. Treat each trade as a noisy equality between the two package sums and mutate every involved player's raw value ([valuation.py:244-305](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:244)).
3. Convert decayed cumulative weekly PAR to an overall rank, map that rank through the same curve, and mutate the same raw value ([valuation.py:307-343](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:307)).
4. Publish the resulting belief directly, with `vorp = max(value - rho, 0)` ([valuation.py:439-469](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:439)).

There is no final normalization, blend, position-specific replacement subtraction, or exponential calibration in the implementation.

## Findings

### 1. Critical: trade fairness has a perfect flat-value solution

For a trade with sides `A` and `B`, the model asks for:

```text
sum(value(A)) = sum(value(B)) - rho * (len(B) - len(A))
```

Set every player value to the same constant `C` and set `rho = C`. Every 1-for-1, 1-for-2, 1-for-3, and general `n`-for-`m` trade is then satisfied exactly. The current code itself now records this fact ([valuation.py:475-496](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:475)).

This has several consequences:

- Lower trade residual is not evidence of a better value curve. The globally perfect curve is flat.
- A random trade holdout does not fix this. The constant solution also scores perfectly on held-out trades.
- ADP is only an initial condition, not a continuing scale constraint. As trade count grows, its influence is washed out.
- Weekly performance has at most 18 update events per player, while the supplied elite players appear in roughly 500-1,600 trades.

The supplied replay demonstrates the result:

| Measure | Intended seed curve | Supplied final replay |
| --- | ---: | ---: |
| Rank-1 value | 10,000 | 4,682 |
| Implied seed-curve rank of top value | 1 | 20.0 |
| Fitted `rho` | 17 at seed rank 160 | 1,445, equivalent to seed rank 49.4 |
| Median value | — | 135, equivalent to seed rank 108.6 |

A negative-control probe using the real updater and 100 ADP-seeded players showed that feeding more arbitrary package equalities simultaneously reduced trade error and destroyed the curve:

| Trades | Top | Median | Cross-player SD | Mean absolute trade gap |
| ---: | ---: | ---: | ---: | ---: |
| 0 | 10,000 | 1,381 | 2,608 | — |
| 1,000 | 3,419 | 2,115 | 317 | 1,112 |
| 5,000 | 2,342 | 2,069 | 83 | 523 |
| 20,000 | 1,915 | 1,870 | 21 | 266 |
| 40,000 | 1,868 | 1,840 | 12 | 220 |

The arbitrary trades are intentionally a negative control, not a model of a rational market. The important result is that the model's health metric improves monotonically as the output becomes useless. A robust model must reject or downweight inconsistent observations and must have independent scale/shape calibration.

The comment suggesting “projecting out the common-mode component” is not enough. Balanced trade rows already have coefficients summing to zero and therefore contain no additive common-mode information. Their effect is to suppress differences along the connected trade graph—a graph-Laplacian smoothing problem. The model needs persistent anchors or shape constraints, not merely common-mode projection.

### 2. Critical: `rho` is only conditionally identified and conflates three concepts

The fixed-point routine estimates `rho` from unbalanced trades after values have already been updated using `rho` ([runner.py:201-260](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/runner.py:201)). Conditional on fixed player values, unbalanced trades identify an offset. Jointly, they do not identify player scale and `rho`, because the constant solution sets both to the same number.

Raising `rho` therefore makes the flat solution easier to reach. In a fixed synthetic trade stream, increasing `rho` from 17 to 2,500 reduced the value spread from 654 to 273 while also reducing mean trade gap from 862 to 322. The fitting objective rewards the contraction.

The value named `rho` currently represents three different ideas that should be separate:

- A consolidation/package offset in the trade likelihood.
- The value of a bench or waiver roster spot.
- A player's position-specific replacement threshold for VORP.

The final item cannot be one global scalar. The model already configures distinct weekly replacement ranks by position and league size ([config.py:21-64](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/config.py:21)), but the published VORP ignores them and subtracts the fitted global `rho` from every position ([valuation.py:439-450](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:439)). This contradicts the design's query-time, league- and position-specific replacement calculation ([modeling.md:179-193](/Users/seankane/github.com/ff-sims-run-model-daily/docs/arch/modeling.md:179)).

### 3. High: evidence weighting runs opposite to explicit recency weighting

Every trade shrinks the involved players' variance, including an outlier ([valuation.py:275-305](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:275)). Smaller variance gives subsequent observations less leverage. Apart from the relatively small per-day drift, early trades therefore matter more and recent trades matter less.

A three-player probe showed the movement caused by the same new trade after increasing amounts of prior trade history:

| Previous identical trades | Movement from new contradictory trade | Resulting SD |
| ---: | ---: | ---: |
| 0 | 1,975 | 1,061 |
| 1 | 1,314 | 949 |
| 10 | 325 | 525 |
| 100 | 35 | 174 |
| 1,000 | 4 | 55 |

The result is also order-dependent. Applying the same three trades forward versus in reverse produced final player values differing by as much as 209 points. A batch optimizer for one snapshot should be invariant to arbitrary row ordering; a temporal model should differ only because of meaningful timestamps and explicit recency weights.

The `sd` output is consequently filter confidence under strong independence assumptions, not an empirically calibrated market uncertainty band. Repeated correlated trades always make the model more confident, and the independent-player approximation omits the covariance introduced by the trades themselves.

### 4. High: the performance signal discards magnitude and reuses cumulative evidence

Weekly performance is converted from PAR magnitude to ordinal rank, and that rank is mapped to a fixed curve value ([valuation.py:324-343](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:324)). Once a player remains first by even a tiny positive cumulative PAR margin, the observation remains 10,000 regardless of whether the margin is 20 points or 0.2 points.

In a deterministic probe, a rank-50 ADP player had one breakout week followed by six zero-point weeks. Their cumulative PAR decayed from 20.0 to 7.54, but their value rose every zero week:

| Observation | Cumulative PAR | Value | SD |
| --- | ---: | ---: | ---: |
| After breakout | 20.00 | 3,752 | 1,044 |
| After 1 zero week | 17.00 | 5,847 | 852 |
| After 3 zero weeks | 12.28 | 7,968 | 596 |
| After 6 zero weeks | 7.54 | 9,032 | 411 |

The cause is not the 0.85 decay itself. It is converting a shrinking magnitude to the same rank-1 observation and repeatedly fusing that correlated cumulative observation as if it were new independent evidence.

There is a second decay bug: only players present in the current score frame have `cum_par` and `games` decayed. A player absent for five weeks retained cumulative PAR of 20.0 in the probe; full weekly decay would have reduced it to 8.87. Bye, injury, inactive, and missing-row semantics therefore affect form incorrectly.

For a trade recommender, expected future production is also more useful than past weekly scoring. Historical PAR should be a feature in a rest-of-season projection, not directly equated with market value.

### 5. High: outliers are measured but still change values and confidence

The current branch calculates an outlier score and movement share, but explicitly does not act on either ([valuation.py:277-305](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:277)). A dump, collusive trade, desperation move, orphan-team cleanup, or data error therefore:

- Moves every involved value.
- Shrinks uncertainty.
- Influences the fitted `rho` if the sides differ in size.

This is materially different from the two-pass behavior public trade-based calculators document, which removes trades with large fitted value differences before producing final values.

### 6. High: the exponential curve is a prior, not the published curve

The configured curve is actually a reasonable first approximation to the requested shape:

| Rank | Seed value |
| ---: | ---: |
| 1 | 10,000 |
| 2 | 9,608 |
| 3 | 9,231 |
| 5 | 8,521 |
| 10 | 6,977 |
| 20 | 4,677 |
| 30 | 3,135 |
| 50 | 1,409 |

It provides several elite players before dropping quickly. The problem is that it is applied only at ADP initialization and performance-rank observation time. The final raw beliefs are not placed back on this or any other calibrated distribution.

Simply rescaling every final value so the top equals 10,000 would hide the symptom but not fix package ratios, ranks, recency, VORP, or uncertainty. The shape must be a persistent part of the estimator or a separately validated final calibration.

### 7. Medium: the written validation objective accepts the degenerate answer

The architecture names held-out trade error as the primary ground-truth metric ([modeling.md:269-281](/Users/seankane/github.com/ff-sims-run-model-daily/docs/arch/modeling.md:269)). Without independent scale constraints, the constant/zero solution wins this metric in both training and holdout data.

The listed external and predictive checks—consensus rank correlation, next-month ADP movement, and future scoring—are not implemented in `analysis/`. The current 107 tests cover mechanics and persistence well, but there is no synthetic recovery test or empirical accuracy gate.

### 8. Medium: a value-only calculator cannot find trades that improve both teams

Equal package values can identify market-fair proposals. They cannot establish that either roster improves. Mutual improvement comes from differing marginal utility:

- Positional needs and starter slots.
- The player displaced from each lineup.
- The player dropped to waivers in an uneven package.
- Rest-of-season schedule and projections.
- Contender versus rebuilding time horizon in keeper/dynasty.

The architecture currently describes trade suggestions as roughly equal total values ([modeling.md:17-23](/Users/seankane/github.com/ff-sims-run-model-daily/docs/arch/modeling.md:17)). That is a fairness filter, not an improvement objective.

## Comparison with an external consensus market

A widely used public trade-value site documents a materially different pipeline:

| External consensus practice | Current model |
| --- | --- |
| Initial optimization across trades | Sequential one-trade belief mutation |
| Remove large-residual trades | Record outliers but apply them fully |
| Regression adjustment for superflex, TEP, PPR, and team count | Exact segmentation by PPR, superflex, team count, and redraft only; no cross-setting regression |
| Per-player implied trade values averaged with explicit recency weights | Old evidence reduces variance, causing later evidence to have less influence |
| Exponential distribution baked into player values | Exponential curve used only for seed/observations |
| Small waiver adjustment kept visible in calculator; documented example is about 425 around dynasty rank 300 | Fitted global `rho = 1,445` used inside training and published as universal VORP threshold |

I also pulled that site's current 10-team PPR superflex redraft values on 2026-07-31. This is a sanity check, not ground truth, and it is not a clean temporal comparison: the supplied replay is a 2025-season model carried through the 2026 offseason, while the consensus source reflects the current 2026 market.

For the 30 players in the supplied output:

- Consensus values were a median 2.13 times higher.
- The multiplier varied from 1.09 to 3.15, so this is not a single scale-factor problem.
- Rank Spearman correlation within those 30 players was 0.665.
- The supplied model's top-10 spread was 1,446 points; the consensus values for those same ten players spanned 3,822.
- The consensus source had Josh Allen at 10,004 and Christian McCaffrey at 6,396; the supplied model had them at 3,461 and 4,682 respectively.

The benchmark supports both diagnoses: the model is compressed and its ordering differs materially. The temporal mismatch means it should not be used to declare individual 2025 rankings wrong.

## Recommended rebuild

### A. Define three outputs instead of one overloaded value

Store these independently:

1. `market_value`: what the trade market appears willing to exchange.
2. `ros_par` or `projected_points`: expected rest-of-season production above position replacement.
3. `market_uncertainty`: empirical dispersion/liquidity of recent implied trade values.

Compute league-specific `replacement_value` and team-specific `marginal_utility` at query time. Do not blend all of these into one latent belief. Their disagreement is useful: a productive player trading below projected utility is precisely a trade target.

### B. Replace recursive trade mutation with a robust anchored snapshot fit

For each valuation date, fit positive player values jointly from a recency window. A useful starting objective is:

```text
minimize over value_i >= floor:

    sum_trade j [
        recency_weight_j
        * league_weight_j
        * Huber(sum(value on A_j) - sum(value on B_j))
    ]
    + lambda_adp * prior_distance(value, current_ADP_prior)
    + lambda_shape * curve_distance(ordered_values, target_curve)
```

Key requirements:

- Normalize the trade loss so adding more rows does not automatically overwhelm one prior per player.
- Use an initial fit, calculate standardized residuals, remove or strongly downweight outliers, then refit.
- Weight explicitly by age; tune a redraft half-life by time-blocked validation.
- Cap or normalize influence by league and player exposure so one hyperactive league or highly liquid player does not create false precision.
- Make the fit deterministic and invariant to input order within a snapshot.
- Anchor at least two parts of the scale, such as top value and waiver/replacement value, or apply an externally calibrated monotone curve after estimating ranks.
- Do not jointly infer the published scale solely from equality residuals.

If a state-space model is eventually desired, fix this observation and identifiability problem first. A Kalman filter or RTS smoother wrapped around the same equality likelihood will reproduce the same flat optimum with more complicated machinery.

### C. Calibrate the curve independently

The current `lambda = 0.04` already provides a reasonable multi-player elite tier, but it should be validated rather than assumed. Fit a monotone curve against explicit anchors:

- Top value near 10,000.
- Several top-tier players within a chosen percentage of the top.
- Starter, flex, and bench anchor ranges.
- Package-trade calibration by trade size.

If the top tier needs a flatter shoulder followed by a faster drop, use a monotone spline or a shifted/stretched exponential such as:

```text
value(rank) = floor + (top - floor) * exp(-k * max(0, rank - tier_width)^p)
```

with `p > 1` for a flatter top shoulder. Fit `tier_width`, `k`, and `p` on validation anchors; do not select them solely by appearance or trade residual.

### D. Treat replacement and waiver cost at query time

For a specific league:

1. Determine replacement separately for QB, RB, WR, and TE from actual starters, flex usage, benches, and free agents.
2. Compute a player's production VORP relative to their position.
3. For an uneven package, model the actual player displaced from the lineup and the actual roster player dropped—not one global `rho`.
4. If a generic calculator has no roster, use an explicitly named league-format waiver adjustment calibrated from the relevant waiver rank.

This restores the distinction between performance replacement, market price, and roster-slot cost.

### E. Build trade suggestions as a Pareto search

For two teams:

1. Enumerate bounded candidate packages, initially 1-for-1, 2-for-1, 1-for-2, and 2-for-2.
2. Filter to proposals within a market-value fairness tolerance adjusted for uncertainty.
3. Apply the trade to both rosters, including drops needed for roster limits.
4. Re-optimize each starting lineup for future weeks.
5. Calculate each team's change in expected points or simulated championship probability.
6. Return Pareto-improving trades first; otherwise clearly label proposals as fair-market but one-sided.

This is the mechanism that can produce “both teams improve,” because it values players at their marginal contribution to each particular roster.

## Implementation sequence

### Phase 0: Lock down evaluation before changing the estimator

- Preserve a known staged bundle as a versioned or reproducibly fetched fixture manifest.
- Add snapshot metrics for curve spread, effective trade weight, per-player gain by time, exposure by league, outlier share, and value concentration.
- Add time-blocked and league-blocked holdouts; never randomly split rows from the same leagues across train and test.
- Establish benchmark snapshots against ADP, future projections, and external consensus values with dates and settings recorded.
- Add a negative-control metric that explicitly rejects a flat curve even when trade residual is excellent.

### Phase 1: Correct market-value inference

- Implement the anchored batch optimizer behind a parallel output table or model-version key.
- Add explicit recency and league weights.
- Add two-pass robust outlier handling.
- Remove fitted `rho` from the latent player-value update.
- Compare old and new outputs on the same frozen bundle before publishing.

### Phase 2: Correct football value and replacement

- Replace cumulative-PAR rank fusion with a separately validated rest-of-season projection or a magnitude-preserving PAR model.
- Decay every tracked player's performance state at every week boundary, including absences.
- Compute position- and league-specific replacement at query time.
- Calibrate uncertainty against actual future error or implied-value dispersion.

### Phase 3: Add roster-aware trade generation

- Compute post-trade lineups and drops.
- Score changes in future expected points and simulation outcomes.
- Search for fair-market, Pareto-improving packages.

## Regression tests the rebuilt model should satisfy

These tests should be written failing against the current implementation and passing against the replacement:

1. A synthetic market with known exponential values is recovered within defined rank and scale tolerances.
2. Adding duplicated or inconsistent trade equalities cannot collapse the published curve or falsely improve the model-health score.
3. The same snapshot trade set produces the same result regardless of row ordering.
4. A later market regime receives more weight than an older regime according to the configured half-life.
5. A severe outlier neither moves values nor decreases uncertainty after the robust refit.
6. One breakout followed by zero-point weeks does not monotonically approach rank-1 value.
7. A missed week decays performance state exactly once for every weekly boundary.
8. QB, RB, WR, and TE VORP use their own league-specific replacement players.
9. Uneven packages account for the correct dropped/waiver player.
10. A generated “both improve” trade increases the selected team-utility metric for both complete rosters.

## Actions to avoid

- Do not merely multiply final values so the top is 10,000.
- Do not tune `lambda` or variance constants against trade MAE while the flat solution remains admissible.
- Do not interpret the fitted `rho` as position-independent VORP.
- Do not treat raw trade count as independent evidence or the current `sd` as calibrated market volatility.
- Do not build the full sparse Kalman/RTS design until the trade likelihood, scale anchors, and validation metric are corrected.

## Bottom line

The current replay infrastructure is useful and the configured seed curve is close to the requested presentation shape. The estimator between those two points is the problem. It converts a large, noisy, time-varying trade graph into repeated equality updates whose best score is achieved by making everyone equal, then publishes that contracted latent state directly.

The most valuable next implementation step is an anchored, robust, recency-weighted batch market estimator running beside the existing model on a frozen staged bundle. Once market value, projected football value, replacement cost, and team utility are separate, the model can support both credible point values and genuinely useful trade ideas.
