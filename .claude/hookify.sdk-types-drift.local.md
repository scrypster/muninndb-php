---
name: sdk-types-drift-guard
enabled: true
event: stop
action: warn
conditions:
  - field: transcript
    operator: regex_match
    pattern: "internal/transport/rest/types\\.go"
---

**[SDK Drift Warning] types.go modified — remember to update SDKs**

`internal/transport/rest/types.go` was edited this session. This file defines the request/response structs that the SDKs are generated from.

If request or response schemas changed, the following SDKs in separate repos need updating:
- **Python SDK** — `sdk/python/` (MuninnDB Python client)
- **Node/TypeScript SDK** — `sdk/node/` (MuninnDB Node client)
- **PHP SDK** — check separate repo

Action items:
1. Review which types changed (added fields, renamed fields, type changes)
2. Update SDK type definitions in each SDK repo
3. Bump SDK version numbers and update changelogs

This is a warning only — SDK repos are separate and cannot be auto-updated here.
