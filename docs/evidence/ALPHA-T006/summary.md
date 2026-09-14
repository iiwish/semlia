# ALPHA-T006 Delivery Summary

## Outcome

Semlia Web Ask is connected to the configured workspace LLM and the canonical T005 semantic
resolver. A question is interpreted through the strict `semlia.ask-interpretation/v1` schema, then
resolved against one selected immutable release under channel `ask`. The UI renders only released
definitions, the validated semantic plan, clarification, explicit resolver refusal or a provider
failure.

The real runtime contains no timer-backed or fixture-backed success path. Fixture mode disables the
composer and explicitly states that it does not call the backend or generate demonstration answers.

## Authority And Privacy

The model cannot choose a release, submit raw SQL, provide source locations or add unknown request
fields. The server injects the supported query schema version and caller-selected release context,
then delegates release selection, authorization and plan validation to the shared resolver.

Agent runs retain an input hash, model, configuration revision, output digest, cost and duration.
Model and resolver steps retain hashes and stable error codes. The raw question and generated prompt
are not persisted in agent runs, steps, SemanticQuery records, audit events, usage events or outbox
events.

## Product Boundary

Successful Ask plans report `executionStatus=not_configured`. The UI states that semantic resolution
completed but no data query ran; it never displays numeric answers, trends or fact rows. Provider
failure reports `未回退`, clarification does not create a semantic plan, and resolver refusal states
that Semlia will not replace refusal with a guess or unpublished knowledge.

Every navigation surface was classified in `route-disclosure-inventory.md`. Real Alpha surfaces stay
operational; fixture-backed, session-only and mixed surfaces show a visible boundary before users can
mistake them for persisted functionality.

## Residual Risk

The live provider proof used a deterministic local OpenAI-compatible server configured through the
same persisted provider/model path as a real endpoint. It proves transport, schema, attribution and
failure handling, but it is not evidence of a production OpenAI account or production model quality.
No warehouse execution adapter is included in Alpha. The production Web bundle remains above Vite's
500 kB advisory threshold.
