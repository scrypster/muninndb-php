---
name: api-spec-drift-guard
enabled: true
event: stop
action: warn
conditions:
  - field: transcript
    operator: regex_match
    pattern: "internal/transport/rest/(server\\.go|admin_handlers\\.go|admin_vault_handlers\\.go|admin_backup_handler\\.go|admin_cluster_handlers\\.go|cluster_handlers\\.go|replication_handlers\\.go|consolidation_handlers\\.go|observability_handler\\.go)"
---

**[API Drift Warning] REST handler modified — was openapi.yaml also updated?**

One or more REST route handler files were edited this session, but the OpenAPI spec may not have been updated.

Files to check:
- `internal/transport/rest/openapi.yaml` — must reflect any new/changed/removed routes

If you added, removed, or changed a route signature:
1. Update `internal/transport/rest/openapi.yaml` with the new/modified path
2. Verify the spec is valid: `npx @redocly/cli lint internal/transport/rest/openapi.yaml`

If this was a non-route change (bug fix, internal logic only), you can ignore this warning.
