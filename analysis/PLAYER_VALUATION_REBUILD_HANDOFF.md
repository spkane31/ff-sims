# Player Valuation Rebuild — Agent Handoff

**Status:** Ready for implementation  
**Background:** [PLAYER_VALUATION_MODEL_REVIEW.md](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/PLAYER_VALUATION_MODEL_REVIEW.md:1)

## Goal

Replace the recursive player-value estimator while preserving the working database, staging, replay, and snapshot pipeline. Produce:

1. A trade-derived `market_value` on a stable exponential 300–10,000 scale.
2. A separate magnitude-preserving `projected_par` signal from ADP and weekly scoring.
3. League-specific positional replacement and roster-drop costs at query time.
4. Trade suggestions that are market-fair and improve projected utility for one or both teams.

Do not tune the existing `rho`, `lambda`, or variance constants. The current equality update admits a perfect flat-value solution ([valuation.py:244-305](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:244)).

## GitHub issue disposition

No GitHub changes have been made. Recommended actions:

| Issue | Action | Reason |
| --- | --- | --- |
| [#94](https://github.com/spkane31/ff-sims/issues/94) weekly stats activity | Close as completed duplicate | Implemented by #118 / PR #120. |
| [#95](https://github.com/spkane31/ff-sims/issues/95) segmented schema | Close as superseded | PR #121/migration 014 implemented the live schema differently; the proposed three-signal schema belongs to the abandoned blend. |
| [#96](https://github.com/spkane31/ff-sims/issues/96) ADP signal | Close as completed/superseded | ADP input and rollups exist via PRs #121, #130, #133, and #157. Keep ADP as a prior, not a stored percentile blend signal. |
| [#97](https://github.com/spkane31/ff-sims/issues/97) additive least squares | Close as obsolete | Its unanchored trade objective has the flat/zero optimum. Replace it with the robust anchored estimator below. |
| [#98](https://github.com/spkane31/ff-sims/issues/98) PAR blend signal | Close as superseded | Performance must preserve magnitude and remain separate from market price. |
| [#99](https://github.com/spkane31/ff-sims/issues/99) calendar blend/Temporal workflow | Close as obsolete | The fixed three-signal blend is abandoned; daily replay scheduling already exists by another route. |
| [#100](https://github.com/spkane31/ff-sims/issues/100) VORP/exponential endpoint | Close as superseded | The product outcome remains, but the `raw_segment_score` formula and value-only suggester are wrong. Implement Work Packages 3–4 below. |
| [#101](https://github.com/spkane31/ff-sims/issues/101) injury multiplier | Close as superseded | Injury belongs in future-point projection and market observations; a hard multiplier would double-count it. |
| [#102](https://github.com/spkane31/ff-sims/issues/102) validation metrics | Close as superseded | Random trade holdout plus trade MSE rewards the flat solution. Use the evaluation contract below. |
| [#103](https://github.com/spkane31/ff-sims/issues/103) Phase 1 epic | Close as abandoned | Its component architecture is superseded by this document. |
| [#104](https://github.com/spkane31/ff-sims/issues/104) Kalman/RTS Phase 2 | Close as premature/obsolete | It wraps the same invalid trade likelihood in a more complex model. Reconsider only after this estimator is validated. |
| [#126](https://github.com/spkane31/ff-sims/issues/126) held-out harness | Close as superseded | Keep the harness idea, but use league/time-block splits and non-degenerate metrics. |
| [#131](https://github.com/spkane31/ff-sims/issues/131) auction/dynasty ADP | Keep open | Valid future data work, independent of the redraft estimator rebuild. Not part of this implementation. |

Suggested closure text: `Superseded by analysis/PLAYER_VALUATION_REBUILD_HANDOFF.md after model diagnosis; the former objective/architecture is no longer the implementation target.`

## Rules for implementation

- Use TDD. Every test below must be observed failing against the old behavior before implementation and passing afterward.
- Keep the existing CLI, staged-bundle format concept, replay boundaries, database transaction, and snapshot publication behavior unless explicitly changed below.
- Implement the new model under a new module and model-version key. Do not silently overwrite the old model until the same frozen bundle has been compared.
- Do not use held-out trade imbalance as the only selection metric.
- Do not implement the Kalman/RTS design in this pass.

## Work Package 1 — Data and evaluation seam

### Changes

1. Add `league_id` to `Trade` in [models.py:6-15](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/models.py:6), select it in [db.py:231-271](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/db.py:231), and persist it in staged Parquet. Bump the bundle schema version.
2. Add `analysis/src/evaluation.py` with:
   - League-blocked holdout: no league may appear in both train and test.
   - Time-blocked holdout: train before cutoff, evaluate after cutoff.
   - Metrics: package MAE, package percentage error, rank recovery, curve anchors, value spread, and external Spearman when a dated benchmark is supplied.
3. Add a flat negative control. Evaluation must mark a constant-value model invalid even if its package MAE is lower.
4. Add `--evaluate-bundle <path>` that never writes to either database.

### Proof

- Two trades from the same league always land in the same split.
- A constant model with `value == rho` fails `curve_valid`, despite zero package error.
- Evaluation with a fake database object performs zero writes.
- A deterministic 60-player synthetic market with known ranks reports Spearman `>= 0.95` when given its true values.

## Work Package 2 — Market estimator and final curve

### Changes

1. Add `analysis/src/market_value.py`; stop using `Valuator.apply_trade()` for the new model.
2. At each snapshot, fit all player market scores jointly from a rolling trade window, warm-started from the previous snapshot:
   - Trade weight: `2 ** (-age_days / 30)` initially.
   - Normalize/cap total weight per league so one league cannot dominate.
   - Keep an ADP prior for every drafted player on every fit; normalize trade loss so raw trade count cannot erase it.
   - Use sparse Huber IRLS. Add SciPy for sparse matrices/solves.
   - First fit → standardized residuals → remove/downweight `|z| > 3` → final fit.
   - Constrain scores non-negative and make snapshot output invariant to input row order.
3. Convert final score order to published values with one explicit calibration:

   ```text
   market_value(rank) = 300 + 9700 * exp(-0.04 * (rank - 1))
   ```

   This is the v1 presentation curve; it is not fitted from trade residuals.
4. Remove fitted `rho` from model training. Keep package/waiver cost for query time only.
5. Persist `model_version`, `market_score`, `market_value`, and empirical recent-trade dispersion. Keep old columns during the comparison period.

### Required failing-then-passing tests

| Test | Expected result |
| --- | --- |
| `test_published_curve_has_required_anchors` | Ranks 1, 5, 20, 50, 100 round to `10000`, `8566`, `4836`, `1666`, `485`. |
| `test_trade_volume_cannot_flatten_published_curve` | After 5,000 deterministic noisy trades, top remains `10000`, rank 5 `8566`, and rank 20 `4836`. Current model's top falls near 2,300. |
| `test_snapshot_fit_is_order_invariant` | Reversing all trade rows changes no fitted value by more than `1e-6`. |
| `test_recent_market_wins` | Equal old/new samples implying 4,000 then 8,000 produce a current estimate closer to 8,000 and above 6,500. |
| `test_one_league_cannot_duplicate_an_outlier_into_consensus` | Duplicating one bad trade 100 times in one league changes any published value by `< 5%`. |
| `test_outlier_does_not_increase_confidence` | A `|z| > 3` trade is excluded/downweighted and cannot reduce reported uncertainty. |

Also run the 60-player synthetic market end to end: final rank Spearman `>= 0.90`, top five all `>= 8,566`, and no value below 300.

## Work Package 3 — Performance, replacement, and uncertainty

### Changes

1. Add `analysis/src/performance.py`. Compute weekly position-relative PAR as now, but preserve its magnitude.
2. Decay every tracked player at every week boundary, including byes, injuries, inactive players, and missing score rows.
3. Compute a projected-performance proxy:

   ```text
   projected_par = (4 * preseason_par + effective_games * recent_mean_par)
                   / (4 + effective_games)
   ```

   Store it separately. It must not mutate `market_value`.
4. Replace `sd` with two named quantities:
   - `market_dispersion`: robust spread of recent implied trade values.
   - `projection_uncertainty`: error band for projected PAR.
5. At query time compute replacement separately for QB/RB/WR/TE from the league's starters, flex usage, rosters, and available players. Remove global `value - rho` VORP from [valuation.py:439-450](/Users/seankane/github.com/ff-sims-run-model-daily/analysis/src/valuation.py:439).

### Required failing-then-passing tests

- One breakout followed by six zero-point weeks decreases `projected_par`; it must not rise toward a 10,000 market value. The old probe rose from 3,752 to 9,032.
- Five missed weeks decay PAR `20.0` to `20 * 0.85**5 == 8.8741` within floating tolerance.
- Applying weekly scores changes `projected_par` but leaves `market_value` unchanged.
- In a fixture where QB replacement value is 2,000 and RB replacement is 800, a 3,000-value QB has VORP 1,000 while a 3,000-value RB has VORP 2,200.
- Repeating the same correlated trade does not make `market_dispersion` approach zero.

## Work Package 4 — Roster-aware trade suggestions

### Changes

1. Keep fairness and improvement separate:
   - `fairness_delta`: difference in package `market_value`, including actual dropped-player cost.
   - `utility_delta`: change in optimized rest-of-season starting-lineup projected points or simulation win probability.
2. Enumerate 1-for-1, 2-for-1, 1-for-2, and 2-for-2 packages between two rosters.
3. Apply roster limits, drops, and post-trade lineup optimization before scoring either team.
4. Return Pareto-improving trades first. Label fair but one-sided trades explicitly.

### Required failing-then-passing test

Use two fixed rosters where Team A has a bench QB worth 5,000 market points and Team B has a bench WR worth 5,100. After swapping them, projected starters improve by `+3.0` for Team A and `+2.0` for Team B. The suggestion must:

- Pass a 10% market-fairness tolerance.
- Include both post-trade lineup changes.
- Report utility deltas `+3.0` and `+2.0`.
- Rank above a market-fair proposal that makes either team worse.

## Definition of done

- All new tests were demonstrated red against old behavior and green against the new model.
- Existing 107 Python tests still pass or were deliberately replaced with equivalent pipeline-contract tests.
- The supplied frozen bundle can run old and new model versions without database writes.
- New output satisfies the curve anchors and synthetic recovery gates above.
- Evaluation reports league-blocked and time-blocked results, plus the flat negative control.
- A database replay publishes the new version atomically without deleting the old comparison snapshots.
- Documentation no longer presents global fitted `rho`, recursive belief `sd`, or held-out trade MAE as VORP, calibrated uncertainty, or sole ground truth.
