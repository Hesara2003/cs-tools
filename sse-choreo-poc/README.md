# sse-choreo-poc

Throwaway component with one job: find out whether Choreo's API gateway
delivers Server-Sent Events incrementally, or buffers the response until the
connection ends. Delete this directory once the question is answered —
nothing here is meant to be kept.

## What it does

- `GET /stream` writes a `tick` event every 2 seconds and a comment-only
  `: ping` heartbeat every 15 seconds, flushing after each write, with
  `X-Accel-Buffering: no` set (same technique
  `apps/csm-portal/backend`'s real stream handler uses).
- `GET /health` is a plain 200, for Choreo's own health checks.

No auth — deliberately public, since the only thing being tested is
transport-level SSE behavior through the gateway, not this repo's auth flow.

## Deploying

1. Push this directory to the repo (its own branch is fine).
2. In the Choreo console, create a new component pointing at
   `sse-choreo-poc/` — the Go buildpack should auto-detect `go.mod`.
3. Deploy to a dev environment and grab its public URL.

## Testing SSE compatibility

Against the **deployed** Choreo URL — not localhost, the whole point is
testing the gateway:

```
curl -N https://<your-choreo-url>/stream
```

`-N` disables curl's own output buffering. Then watch:

- **Working**: `event: tick` lines appear roughly every 2 seconds, in real
  time, matching the `sending tick N at ...` lines in Choreo's log viewer
  for this component.
- **Broken (gateway buffering)**: nothing appears for a long stretch, then
  everything arrives in one burst — either once some buffer threshold is
  hit, or only when the connection closes/times out. Compare the
  client-side arrival time against the server-side "sending tick N at ..."
  log timestamp for the same `n` — a large gap between the two confirms the
  gateway is buffering, not this code.

Also worth checking while it's up:

- How long a connection survives before Choreo forcibly closes it — some
  gateways cap total/idle connection duration regardless of heartbeats.
- Whether the browser's native `EventSource` (not just curl) behaves the
  same way, since that's the real client-side story.
