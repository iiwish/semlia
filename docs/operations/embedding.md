# Embedding Index

Embedding rebuilds require pgvector and an enabled default OpenAI-compatible embedding setting. The deployment environment must contain the configured credential environment variable with exactly the persisted credential revision. Missing configuration reports `not_configured`; lexical search remains available.

Migration 19 installs vector storage when the server already provides the pgvector extension. A standard PostgreSQL server still migrates metadata successfully. To enable vectors later, an administrator installs pgvector for the existing PostgreSQL major version and runs `psql "$SEMLIA_DATABASE_URL" -v ON_ERROR_STOP=1 -f deploy/local/enable-pgvector.sql`. The runtime never performs extension/schema DDL in a user request. Capability checks require both the extension and vector column. Do not change a PostgreSQL 18 data volume to a PostgreSQL 17 image or change its operating-system/collation base without a separate supported migration.

Rebuilds capture an immutable published release and bounded asset address/name/title/description chunks. They pin endpoint, provider, model, dimension, credential revision and chunking version. Existing active search is not changed by edits to model defaults. Every checkpoint and activation requires the current worker lease; cancellation or terminal failure preserves the previous active generation. Search explicitly reports vector or lexical mode and uses only currently published revisions that the caller can read.

Provider transport tests use deterministic HTTP servers and are not proof of an external provider's availability, pricing or model quality.
