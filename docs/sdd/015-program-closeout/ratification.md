# Owner ratification packet

015 does not edit ADR status bytes. The owner ratifies by changing
`docs/adr/**` on a later change.

| ADR | File | Status now | Code on main | Owner action |
|---|---|---|---|---|
| ADR-021 | `docs/adr/ADR-021-clean-exit.md` | proposed | yes | accept or reject |
| ADR-022 | `docs/adr/ADR-022-preflight.md` | proposed | yes | accept or reject |
| ADR-026 | `docs/adr/ADR-026-audit-hardening-delivery.md` | proposed | yes | accept or reject |
| ADR-027 | `docs/adr/ADR-027-owned-storage-lifecycle.md` | proposed | yes | accept or reject |

I8 says no ADR stays `proposed` with code in `main`. That flip is
owner-only (PROGRAM ratification row). This packet exists so the gap is
visible and is not papered over by a fake close.
