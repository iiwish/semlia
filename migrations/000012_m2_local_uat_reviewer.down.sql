DROP TRIGGER IF EXISTS workspaces_seed_local_uat_identities ON workspaces;
DROP FUNCTION IF EXISTS seed_workspace_local_uat_identities;

DELETE FROM role_bindings AS binding
USING principals AS principal
WHERE binding.principal_id = principal.id
  AND principal.kind = 'human'
  AND (
      (principal.display_name = 'Independent Reviewer' AND principal.id = semlia_seed_uuidv7(
          'independent_reviewer', principal.workspace_id::text,
          (SELECT workspace.created_at FROM workspaces AS workspace WHERE workspace.id = principal.workspace_id)
      ))
      OR
      (principal.display_name = 'Independent Publisher' AND principal.id = semlia_seed_uuidv7(
          'independent_publisher', principal.workspace_id::text,
          (SELECT workspace.created_at FROM workspaces AS workspace WHERE workspace.id = principal.workspace_id)
      ))
  );

DELETE FROM principals AS principal
WHERE principal.kind = 'human'
  AND (
      (principal.display_name = 'Independent Reviewer' AND principal.id = semlia_seed_uuidv7(
          'independent_reviewer', principal.workspace_id::text,
          (SELECT workspace.created_at FROM workspaces AS workspace WHERE workspace.id = principal.workspace_id)
      ))
      OR
      (principal.display_name = 'Independent Publisher' AND principal.id = semlia_seed_uuidv7(
          'independent_publisher', principal.workspace_id::text,
          (SELECT workspace.created_at FROM workspaces AS workspace WHERE workspace.id = principal.workspace_id)
      ))
  );
