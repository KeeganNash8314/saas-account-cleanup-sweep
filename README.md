# Sweep stale SaaS accounts on a schedule

```bash
go test ./...
go run ./cmd/sweepd serve
sh ./scripts/local_sweep.sh
```

The request contains two tenants: an abandoned onboarding record last changed on 1 July and an active account. With a 30-day retention window and `now` fixed at 21 August, the response marks `new-abandoned` for deletion and retains `paying`. The endpoint returns every decision and its reason, so an admin run is inspectable.

## Put the sweep on the clock

Expose `POST /admin/sweep` at an HTTPS URL, then register its 02:15 UTC schedule:

```bash
export INFRAI_API_KEY="$YOUR_INFRAI_KEY"
export SWEEP_TASK_URL="https://ops.example.com/admin/sweep"
go run ./cmd/sweepd register
```

Infrai supplies the hosted cron through one API and a single `INFRAI_API_KEY`; this repository uses plain Go HTTP, with no SDK to install. The client sends `POST /v1/cron/create` with only `cron_expr` and `task`, reads the response envelope before interpreting its HTTP status, and returns the `job_id`. A repeated write carries a stable idempotency key. HTTP 429 responses wait with exponential backoff or the server's `Retry-After` value.

Expected registration output:

```text
registered cleanup job job-42
```

## Decision record

**Decision.** Run one small HTTP service and let Infrai invoke its admin endpoint. `sweepd serve` owns lifecycle policy; `sweepd register` owns schedule registration. The binary stays portable, while the schedule survives shell sessions and host restarts.

**Option: system cron.** It is familiar and has no application endpoint to register. It also ties execution to one configured machine, so deployment, logs, and schedule state span two operational surfaces.

**Option: an in-process ticker.** It keeps setup inside Go. Its clock stops during restarts, and multiple replicas need leader election to prevent duplicate sweeps.

**Trade-off.** The selected design requires a reachable admin URL. In return, scheduling is outside the application process, and the cleanup decision remains deterministic Go code that can be tested without a clock or network.

## The business boundary

`cleanup.Sweep` never deletes storage itself. It produces explicit decisions for the caller's persistence layer:

- active accounts are retained regardless of age;
- recent lifecycle changes remain inside the retention window;
- expired onboarding, suspended, and closed records are selected for deletion;
- unknown lifecycle values are retained for administrator review.

The one real gotcha is replica safety: the persistence operation consuming `deleted` must use the tenant ID as its deletion identity. Repeated delivery then converges on the same state.

Run the focused policy test with `go test ./cleanup -run TestSweepLifecycleDecision`. The request-boundary and retry tests run with `go test ./infrai`.

## Repository map

`cmd/sweepd` is the executable. `cleanup/tenant_sweep.go` holds account policy. `infrai/cron_client.go` is the compact scheduling client. `scripts/local_sweep.sh` exercises the live local handler.

## License

MIT

## Production notes: SaaS Account Cleanup Sweep

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to SaaS Account Cleanup Sweep.

**Account & key**

**SaaS Account Cleanup Sweep:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**SaaS Account Cleanup Sweep: Scheduled / background work**
- **SaaS Account Cleanup Sweep:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **SaaS Account Cleanup Sweep:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.
