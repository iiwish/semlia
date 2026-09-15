# FMB-T007 Sprint A001

Status: Incomplete; frozen handoff, not accepted.

Window: 2026-09-05 07:45:34-10:45:34 UTC. Implementation cutoff: 10:20:34 UTC.

The founder explicitly requested the third three-hour sprint. Confirmed Full-Menu Beta
spec/TDR/data-model/task graph govern this attempt. T002/T004/T005/T006 have passing local
implementation and review evidence; external-provider and hosted evidence remains separate.

One worker owns implementation; root owns baseline, review, Docker preview, final gates
and evidence integration. No concurrent implementation, delegation by workers, commit,
push, destructive cleanup or external deployment is authorized.

Preserve the existing dirty worktree and migration1-20. The baseline is captured before
worker dispatch. T006's frozen patch SHA-256 is
`d5bcfd0df7b77beb9e770fb29cf5dac9d4d4c8fe34e6b9d65a1c9b887dc1152f`.

Packet aligns its MCP path with the existing adapter and permits the minimum credential
and UI integration needed for explicit semantic.execute opt-in. Existing read-only
credentials must not acquire execution rights. Publication changes, if required, only
freeze exact physical provenance; legacy releases without it fail closed for execution.

Required proof: typed aggregate SQL compilation, current authorization before connection,
dedicated read-only source, bounds/cancellation/concurrency/restart, metadata-only durable
state, channel/browser parity, migration/release artifacts and an honest hosted matrix.

The worker stopped around08:10 UTC due to an execution quota error and resumed at10:30
only for handoff and bounded validation cleanup. Root verified the frozen core, retained
the failing full source gate, and independently closed a fixed-time Operations test lease
failure. Publication, formal joins, application/channel/UI wiring and execution security
tests remain incomplete. No release or rebuilt preview is produced. The three-hour goal
is not achieved; its scope is not reduced to the partial implementation.
