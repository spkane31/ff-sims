# Worker deployment versioning

The worker host (`deploy/worker-host/`) is the sole fleet that runs
`backend/cmd/worker` and the Python ESPN worker (`workers/espn`), against the same
Temporal Cloud namespace as production. DigitalOcean only builds/serves `cmd/server`
(the API) — it no longer runs any Temporal worker at all. A systemd timer on the host
polls `origin/main` and rebuilds/resyncs + restarts each worker when there's a new
relevant commit; a failed build/sync leaves the previous binary/deps running
indefinitely.

This replaced an earlier two-fleet setup (DigitalOcean + a Raspberry Pi, both running
`cmd/worker`) that needed Worker Deployment Versioning to prevent stale code on one
fleet from racing newer code on the other. The versioning machinery below stayed in
place through that cutover — it's mechanically simpler with one fleet (no promotion
race is possible), but still useful for safe rollback and for build-ID tracking.

All six workers register with Worker Deployment Versioning, `Pinned` behavior, under the
deployment name `ff-sims-worker`. Every workflow in this codebase is short-lived (seconds
to minutes), so an execution started on version N always finishes on version N — there's
no need for version-transition patching.

- `buildID` (`backend/cmd/worker/main.go`) is set via `-ldflags -X main.buildID=<git short
  SHA>` in the worker host's build paths (`deploy/worker-host/deploy.sh` and
  `deploy/worker-host/setup.sh`) — the only place `cmd/worker` is built.
- `promoteOnStart` (`backend/cmd/worker/main.go`) marks a build as the one that should
  promote itself to the deployment's Current Version on startup. It's a build-time flag,
  not an env var: `deploy/worker-host/{deploy,setup}.sh` set `-X
  main.promoteOnStart=true`; the source default is `"false"` so an ad hoc local build
  (e.g. a dev running `cmd/worker` without those ldflags) joins the deployment instead of
  fighting to promote. This used to be the env var `TEMPORAL_PROMOTE_ON_START`, set only
  via DigitalOcean's App Platform config — a fact about which fleet promotes that lived
  entirely outside this repo. Baking it into the build removed that dependency, the same
  way `buildID` no longer depends on an externally-passed `GIT_SHA`.
- When `promoteOnStart` is `"true"`, the worker retries `SetCurrentVersion` with capped
  backoff until it succeeds (the version isn't registered until a worker has polled at
  least once). It no longer gives up after a fixed window — a single missed attempt used
  to mean the deployment never got a Current Version at all, so no new workflow execution
  on any task queue could ever be assigned to a worker.
- `deploy/worker-host/deploy.sh` re-execs itself after `git reset --hard` instead of
  continuing in the same process. Bash had already parsed `build_worker`'s old body
  before the reset rewrote the file on disk, so a commit that changes `build_worker`
  itself (like the one that added `-ldflags` here) would otherwise be applied with the
  stale, pre-pull logic on the very cycle that introduced it.

## Checking version status

```
temporal worker deployment describe --name ff-sims-worker
```

Shows the current version, any ramping version, and every version with active pollers.

## Manually promoting a version

Normally only the worker host (built with `promoteOnStart=true`) promotes on startup. To
promote by hand (e.g. to force a rollback to a previous build that's still draining):

```
temporal worker deployment set-current-version \
  --deployment-name ff-sims-worker --build-id <sha>
```

## Finding workflows stuck on an old version

```
temporal workflow list --query \
  'TemporalWorkerDeploymentVersion = "ff-sims-worker:<sha>" AND ExecutionStatus = "Running"'
```

## Known edge case

A pinned workflow whose version has no live worker (e.g. that version was fully
decommissioned) waits until a worker for that version returns, or until the workflow is
terminated. Since every workflow here completes in seconds to minutes, terminating a
stuck execution and letting the next schedule fire is an acceptable recovery — there's no
need to bring back an old binary just to drain it.
