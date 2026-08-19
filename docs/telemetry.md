# Telemetry

Local, sanitized, off by `JACU_TELEMETRY=off`. Runtime state lives under `.git/jacu`; user-level telemetry lives under the user state directory.

Retention, ownership and deletion are defined by the [storage specification](sdd/specs/storage/spec.md) and [ADR-027](adr/ADR-027-owned-storage-lifecycle.md). The rotation, retention and privacy contract is the [telemetry specification](sdd/specs/telemetry/spec.md), with rationale in [ADR-018](adr/ADR-018-local-telemetry.md).

No `stats` number may be read as a gain until the owner-only G-T baseline exists.
