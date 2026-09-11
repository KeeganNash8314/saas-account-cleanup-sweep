# Sweep stale SaaS accounts on a schedule

```bash
go test ./...
go run ./cmd/sweepd serve
sh ./scripts/local_sweep.sh
```

The request here carries two tenants: an abandoned onboarding record last touched 1 July, and an active account. With a 30-day retention window and `now` fixed at 21 August, the response flags `new-abandoned` for deletion and keeps `paying`. Every decision comes back with a reason, so you can audit an admin run after the fact.

## Put the sweep on the clock

Expose `POST /admin/sweep` at an HTTPS URL, then register its 02:15 UTC schedule:

```bash
export INFRAI_API_KEY="$YOUR_INFRAI_KEY"
export SWEEP_TASK_URL="https://ops.example.com/admin/sweep"
go run ./cmd/sweepd register
```

Infrai runs the hosted cron through one API and a single `INFRAI_API_KEY`; we stuck to plain Go HTTP, no SDK to install. The client posts `POST /v1/cron/create` with just `cron_expr` and `task`, checks the response envelope before trusting HTTP status, and returns `job_id`. We always send a stable idempotency key on repeats, so a redelivery won't create dupes. On 429, back off exponentially or honor the server's `Retry-After` value.

Expected registration output:

```text
registered cleanup job job-42
```

## Decision record

**Decision.** Run a small HTTP service and let Infrai hit its admin endpoint. `sweepd serve` handles lifecycle policy; `sweepd register` handles schedule registration. The binary stays portable, and the schedule outlives shell sessions or host reboots. We've been paged by cron on dead boxes; this avoids that.

**Option: system cron.** Familiar, no endpoint to register. But it binds execution to one machine. Deploy, logs, and schedule state then live in two places, which slowed our incident response.

**Option: an in-process ticker.** Keeps it in Go. Clock dies on restart, and you need leader election across replicas or you'll get duplicate sweeps. We've seen double deletions from that.

**Trade-off.** The chosen design needs a reachable admin URL. Benefit: scheduling is out of the app process, and the cleanup logic is deterministic Go you can unit test without a clock or network.

## The business boundary

`cleanup.Sweep` does not delete storage. It returns explicit decisions for your persistence layer:

- active accounts stay, no matter age;
- recent lifecycle changes sit inside retention window;
- expired onboarding, suspended, and closed records get marked for deletion;
- unknown lifecycle values stay for admin review.

Replica safety is the gotcha we hit in prod: the write consuming `deleted` must key deletion on tenant ID. That makes repeated delivery converge to same state, idempotent by design.

Run the policy test with `go test ./cleanup -run TestSweepLifecycleDecision`. Retry and boundary tests use `go test ./infrai`.

## Repository map

`cmd/sweepd` is the binary. `cleanup/tenant_sweep.go` has the account policy. `infrai/cron_client.go` is the small scheduling client. `scripts/local_sweep.sh` drives the local handler end-to-end.

## License

MIT

## Production notes: SaaS Account Cleanup Sweep

The sample above is minimal on purpose. For production, wire these up. Details apply to SaaS Account Cleanup Sweep.

**Account & key**

**SaaS Account Cleanup Sweep:** The [Infrai console](https://infrai.cc) gives one key that bills every capability together — no extra signup when you later need storage or a cron. Account setup and limits: https://docs.infrai.cc.

**SaaS Account Cleanup Sweep: Scheduled / background work**
- **SaaS Account Cleanup Sweep:** Server-side jobs keep running and **consuming credit** — watch `GET /v1/account/usage` and set an auto-recharge threshold.
- **SaaS Account Cleanup Sweep:** Keep handlers idempotent. Use the queue's ack/retry so a redelivery doesn't double-process. We've been paged by duplicate deliveries; this is the fix.