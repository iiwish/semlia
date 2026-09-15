-- Isolated synthetic QA database only. Supply workspace_uuid for a workspace
-- created through the real API with local-UAT identities already provisioned.
BEGIN;
INSERT INTO semantic_assets(id,workspace_id,namespace,key,asset_type,lifecycle_state)
VALUES ('10000000-0000-7000-8000-000000000001', :'workspace_uuid','qa','revenue','metric','active');
INSERT INTO asset_revisions(id,workspace_id,asset_id,sequence,schema_version,content_digest,content,created_by)
VALUES ('10000000-0000-7000-8000-000000000002', :'workspace_uuid','10000000-0000-7000-8000-000000000001',1,'1.0.0','sha256:a51f5557aa32572112af196e724f3311d78f6d34ca01c5b257953bfaa70b77c3','{"name":"Revenue","description":"Synthetic released revenue for embedding transport QA"}','local-author');
INSERT INTO releases(id,workspace_id,sequence,manifest_digest,published_by,published_at)
VALUES ('10000000-0000-7000-8000-000000000003', :'workspace_uuid',1,'sha256:b5db704ec89a0b4def90cc8b440fa73631e696fbee7a81c82c79470214b42671','local-publisher',clock_timestamp());
INSERT INTO release_assets(workspace_id,release_id,asset_id,revision_id,position)
VALUES (:'workspace_uuid','10000000-0000-7000-8000-000000000003','10000000-0000-7000-8000-000000000001','10000000-0000-7000-8000-000000000002',1);
COMMIT;
