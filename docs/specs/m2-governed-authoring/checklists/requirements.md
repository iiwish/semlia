# M2 Requirements Checklist

## T001 Entry

- [x] D6 and D7 verdicts recorded in `docs/specs/m2-governed-authoring/analysis.md`.
- [ ] Packet `docs/specs/m2-governed-authoring/packets/T001.yaml` is Ready.
- [ ] Principals, roles, bindings and authorization events migrated per data-model.md.
- [ ] Deny-by-default enforcement proven with stable reason codes.
- [ ] M1 catalog behavior unchanged; `make contracts-check` passes.

## Milestone Requirements

- [x] Decision register D1–D8 confirmed by the founder before dependent packets open.
- [ ] Governed loop "AI 提案、验证、人类审核、发布、回滚" completes on real PostgreSQL data.
- [ ] All AI writes carry agent-run attribution per SSOT §8.6.
- [ ] Risk and policy decisions are recomputable on identical inputs.
- [ ] Separation of duties enforced server-side on protected scopes (FR-007).
- [ ] Releases are immutable; rollback is a new release per decision D8.
- [ ] G0/G1 levels only; new workspaces default to G1 (SSOT §8.1).
- [ ] Authoring surfaces use real APIs; future-milestone surfaces stay preview (ADR-0002 §7).
- [ ] Web e2e passes at 1440x900 and 1024x768; `make check-source` and `make check-smoke` pass.
