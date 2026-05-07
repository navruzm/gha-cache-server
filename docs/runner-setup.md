---
title: Runner Setup
outline: [2, 3]
---

# Pointing runners at your cache server

The GitHub Actions runner overwrites `ACTIONS_RESULTS_URL` on every step from the signed job message it receives, so you can't override it from a workflow. Three approaches exist; the **preload** option is recommended.

## Recommended: Node.js preload

`actions/cache@v4` reads `process.env.ACTIONS_RESULTS_URL` at runtime. By setting `NODE_OPTIONS=--require=/path/to/preload.js` in the runner's host environment, every Node-based action loads a preload script before its own code — including `actions/cache`. The preload mutates the env var in-process; the runner never gets a chance to put it back.

The preload (in this repo at `runner/preload.js`):

```js
'use strict'
if (process.env.OVERRIDE_ACTIONS_RESULTS_URL) {
  process.env.ACTIONS_RESULTS_URL = process.env.OVERRIDE_ACTIONS_RESULTS_URL
}
```

You then set three env vars on the runner:
- `NODE_OPTIONS=--require=/opt/cache-redirect.js`
- `OVERRIDE_ACTIONS_RESULTS_URL=https://cache.example.internal/` (your cache server, with trailing slash)
- `ACTIONS_CACHE_SERVICE_V2=true`

### systemd-installed self-hosted runner

Drop the preload to `/opt/cache-redirect.js`, then create an override:

```bash
sudo install -m 0644 runner/preload.js /opt/cache-redirect.js
sudo systemctl edit actions.runner.<scope>.<name>.service
```

Add:

```ini
[Service]
Environment=NODE_OPTIONS=--require=/opt/cache-redirect.js
Environment=OVERRIDE_ACTIONS_RESULTS_URL=https://cache.example.internal/
Environment=ACTIONS_CACHE_SERVICE_V2=true
```

Reload and restart:

```bash
sudo systemctl daemon-reload
sudo systemctl restart actions.runner.<scope>.<name>.service
```

### Docker — wrapper image

`runner/Dockerfile` in this repo extends the official runner image with the preload baked in:

```dockerfile
ARG BASE=ghcr.io/actions/actions-runner:latest
FROM ${BASE}
COPY preload.js /opt/cache-redirect.js
ENV NODE_OPTIONS=--require=/opt/cache-redirect.js \
    ACTIONS_CACHE_SERVICE_V2=true
```

Build and run, supplying your cache URL at runtime:

```bash
docker build -t my-runner runner/
docker run -d \
  -e OVERRIDE_ACTIONS_RESULTS_URL=https://cache.example.internal/ \
  -e GITHUB_URL=https://github.com/<org> \
  -e RUNNER_TOKEN=<token> \
  my-runner
```

### actions-runner-controller (ARC)

See `runner/runner-deployment.example.yaml`. Two pieces:

1. A `ConfigMap` containing the preload (mounted into the pod).
2. An `AutoscalingRunnerSet` (or `RunnerDeployment` for older ARC) whose template sets the env vars and mounts the ConfigMap at `/opt/cache-redirect.js`.

Apply with:

```bash
kubectl apply -f runner/runner-deployment.example.yaml
```

## Caveat: container actions

The preload only runs inside the runner's own Node process. Container actions (`uses: docker://...` or `Dockerfile`-based actions) run in their own image, with their own Node. The preload does not propagate. For `actions/cache@v4` this is fine — it's a JavaScript action, not a container action — but if you rely on a third-party container action that talks to the cache, that traffic still goes to GitHub.

If that matters to you, fall back to the binary patch or run a forked runner.

## Alternative 1: Forked runner

The [falcondev-oss runner fork](https://github.com/falcondev-oss/github-actions-runner) reads `CUSTOM_ACTIONS_RESULTS_URL` and uses it instead of the runner-supplied URL. ~6 lines of change in two C# files. The downside is owning a downstream fork and rebasing on every upstream release.

## Alternative 2: Binary patch

Replace the literal string `ACTIONS_RESULTS_URL` with `ACTIONS_RESULTS_ORL` inside `Runner.Worker.dll` so the runner overwrites a dummy variable, leaving the real one alone:

```bash
sed -i 's/\x41\x00\x43\x00\x54\x00\x49\x00\x4F\x00\x4E\x00\x53\x00\x5F\x00\x52\x00\x45\x00\x53\x00\x55\x00\x4C\x00\x54\x00\x53\x00\x5F\x00\x55\x00\x52\x00\x4C\x00/\x41\x00\x43\x00\x54\x00\x49\x00\x4F\x00\x4E\x00\x53\x00\x5F\x00\x52\x00\x45\x00\x53\x00\x55\x00\x4C\x00\x54\x00\x53\x00\x5F\x00\x4F\x00\x52\x00\x4C\x00/g' /home/runner/bin/Runner.Worker.dll
```

After the patch you can set `ACTIONS_RESULTS_URL=https://cache.example.internal/` directly in the runner env — the runner now overwrites the renamed `ACTIONS_RESULTS_ORL` instead. Fragile across runner upgrades; needs re-applying every time.

## Verifying the redirect

Run a test job and check the cache server logs. You should see `POST /twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry` and follow-up `PUT /devstoreaccount1/upload/<id>` requests. If you only see catch-all proxy traffic, the override isn't taking effect.

A quick sanity check from inside a runner shell:

```bash
node -e "console.log(process.env.ACTIONS_RESULTS_URL)"
```

This won't reflect the runner-time value (the preload runs only when the runner spawns a child for an action), but it should print your override URL since the env is set on the host.
