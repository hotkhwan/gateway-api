# KControl Registry — Phase A.1 Strict Mode Flag

**Date:** 2026-05-18
**Status:** Code shipped; flag default off — operator flip after backfill stable ≥ 1 wk
**Canonical Contract:** [`klynx-api/docs/contracts/kcontrol-gw-managed-registry.md`](../../../klynx/klynx-api/docs/contracts/kcontrol-gw-managed-registry.md) §5.2
**Companion Phase A:** [`docs/plan/done/kctrl-registry-phase-a.md`](done/kctrl-registry-phase-a.md) (gateway-api 3.13.0)

---

## 1. Scope

Phase A shipped with `kctrlsubmsg` in **compat mode** (FORWARD on miss). Phase A.1 adds the fourth branch from contract §5.2:

```text
ROW NOT FOUND AND no kctrl_registry row has been written in the last 5 minutes
   → DROP (don't forward; the registry is supposed to be fully populated after backfill)
```

The "no recent write" guard protects against dropping devices mid-backfill — if writes are still arriving on the registry, the strict-drop is suspended.

### In Scope

- Service-level `strictMode bool` flag and `lastWriteAt` watermark
- Watermark bumped on every successful `Upsert` (and recorded as `serviceStartedAt` on boot so the strict drop is suppressed during the warm-up window before any writes arrive)
- `Decide()` returns `ActionDrop` for **strict + ROW NOT FOUND + lastWriteAt > 5min ago**
- Env: `KCTRL_REGISTRY_STRICT_MODE=true` (default `false`)
- Tests: cover all four combinations (off / on+recent / on+stale / on+no-rows-yet boot grace)
- Version bump 3.13.0 → 3.13.1

### Out of Scope

- Per-replica synchronisation of `lastWriteAt` (each gateway-api pod tracks its own watermark; staleness is local). The 5-minute window is generous enough that single-pod drift is negligible in practice.
- Drift dashboards / metrics on the count of strict-mode drops (acceptable to surface via existing kctrlsubmsg log lines; alerting is out of scope until ops sees real traffic).

### Success Criteria

- Default deploy of 3.13.1 behaves identically to 3.13.0 (`KCTRL_REGISTRY_STRICT_MODE` unset → compat).
- With the flag flipped and the registry quiet for >5 minutes, MQTT messages from unknown hwIds are dropped at the gateway — no Kafka write, log line `kctrlsubmsg: strict mode — unknown hwId dropped`.
- A klynx-api backfill burst (PATCHes within the last 5 minutes) suspends strict-drop — unknowns get FORWARDED through during the burst window.

---

## 2. Rollout

1. Deploy 3.13.1 — flag off, no behaviour change.
2. After klynx-api backfill has populated the registry and prod is steady ≥ 1 week with zero drift incidents reported by `GET /admin/system/kctrlRegistryDrift`, ops sets `KCTRL_REGISTRY_STRICT_MODE=true` in the gateway-api dev env, watches for 24h.
3. Same flip in staging, then prod.
4. After Phase A.1 stable ≥ 1 week, klynx-api Phase D removes the Layer-1 resolver.

Rollback: set `KCTRL_REGISTRY_STRICT_MODE=false` and restart. No data migration; the flag is read at service-init time only.

---

## 3. Implementation Notes

- `lastWriteAt` is an `atomic.Int64` (UnixNano). Initialised to `time.Now()` at `NewService` to give a 5-minute grace window after boot (during which strict-drop is suppressed even on first MQTT message from an unknown hwId).
- Strict mode is read once at `NewService` time from `os.Getenv("KCTRL_REGISTRY_STRICT_MODE")`. Changing the env requires a process restart — intentional, to avoid mid-flight behaviour switches.
- The strict-drop log line includes hwId + topic + `lastWriteAge` so operators can correlate with the registry write rate.
