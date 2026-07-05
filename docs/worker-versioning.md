# Worker deployment versioning

The DigitalOcean app and the Raspberry Pi (`deploy/raspberry-pi/`) run the same
`backend/cmd/worker` binary against the same Temporal Cloud namespace, but deploy at
different times — the Pi polls `origin/main` and rebuilds minutes after DigitalOcean
ships, and a failed Pi build leaves the old binary running indefinitely. Without worker
versioning, stale code polling the shared task queues causes non-determinism errors on
workflows started by newer code.

All six workers register with Worker Deployment Versioning, `Pinned` behavior, under the
deployment name `ff-sims-worker`. Every workflow in this codebase is short-lived (seconds
to minutes), so an execution started on version N always finishes on version N — there's
no need for version-transition patching.

- `buildID` (`backend/cmd/worker/main.go`) is set via `-ldflags -X main.buildID=<git short
  SHA>` in all three build paths (Dockerfile, `deploy/raspberry-pi/deploy.sh`, and
  `deploy/raspberry-pi/setup.sh`). Both fleets built from the same commit produce the
  identical build ID and share one deployment version. The Dockerfile computes the SHA
  itself from the `.git` directory in the build context when no `GIT_SHA` build-arg is
  passed — DigitalOcean's App Platform build doesn't pass one, and previously that meant
  the DO image silently shipped as build `unknown`.
- `promoteOnStart` (`backend/cmd/worker/main.go`) marks the fleet that should promote its
  build to the deployment's Current Version on startup. It's a build-time flag, not an env
  var: the Dockerfile sets `-X main.promoteOnStart=true` unconditionally (it only ever
  builds the DigitalOcean image), while `deploy/raspberry-pi/{deploy,setup}.sh` never set
  it, so Pi builds keep the source default `"false"`. This used to be the env var
  `TEMPORAL_PROMOTE_ON_START`, set only via DigitalOcean's App Platform config — a fact
  about which fleet promotes that lived entirely outside this repo. Baking it into the
  build removes that dependency, the same way `buildID` no longer depends on an
  externally-passed `GIT_SHA`.
- When `promoteOnStart` is `"true"`, the worker retries `SetCurrentVersion` with capped
  backoff until it succeeds (the version isn't registered until a worker has polled at
  least once). It no longer gives up after a fixed window — a single missed attempt used
  to mean the deployment never got a Current Version at all, so no new workflow execution
  on any task queue could ever be assigned to a worker.
- Flow: DO deploys SHA X, promotes X as current, and the Pi self-updates to X a few
  minutes later and shares the work. If the Pi's build fails, it keeps draining
  workflows pinned to its old version and receives no new-version work — no NDEs.
- `deploy/raspberry-pi/deploy.sh` re-execs itself after `git reset --hard` instead of
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

Normally only the DigitalOcean worker (built with `promoteOnStart=true`) promotes on
startup. To promote by hand (e.g. to force a rollback to a previous build that's still
draining):

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
