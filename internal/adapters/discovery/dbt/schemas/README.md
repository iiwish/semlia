# Embedded dbt Artifact Schemas

These deterministic gzip files are the unmodified published JSON schemas used by the T003 dbt adapter.

| File | Source | SHA-256 of gzip file |
| --- | --- | --- |
| `manifest-v12.json.gz` | `https://schemas.getdbt.com/dbt/manifest/v12.json` | `bbe3ef98aa87e33e7c26401a4cdcb48d22b68655ecbbf5a8c7df8e2ba5852df9` |
| `catalog-v1.json.gz` | `https://schemas.getdbt.com/dbt/catalog/v1.json` | `2b7556f5c30cdaf581f0078aa9d1417d0d05f79de41ace3a6219a2135d03fac5` |

The schemas were retrieved on 2026-09-02 and compressed with `gzip -n -9`. Updating an accepted artifact version requires a reviewed adapter-version change, new fixtures and refreshed checksums.
