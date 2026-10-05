# Codebase Consistency Audit

Use when asked to audit the codebase, flags, documentation, tests, targets, handler splits, oplog coverage, or Web API consistency. This topic is read-only: report findings without modifying files.

## Evidence Rules

- Give every finding a file path, line number, and direct evidence.
- Search before deciding so naming differences do not become false positives.
- Base conclusions on current code and configuration rather than stale documentation or memory.
- When the user names a dimension, inspect only that scope. Run every dimension only for a full audit.

## Dimensions

### CLI Flags

Compare flag, usage, and argument definitions in `cmd/skillshare/*.go` with `website/docs/reference/commands/*.md`:

- `UNDOCUMENTED`: present in code but absent from documentation.
- `STALE`: documented but absent from code, or behavior has changed.
- `OK`: name, mode, default, and semantics agree.

### Specifications and Tests

- For completed `specs/`, verify the implementation and testable acceptance criteria.
- For each command handler, inspect unit and integration coverage. A matching filename alone does not prove coverage.
- Use `IMPLEMENTED/MISMATCH/PENDING` and `COVERED/PARTIAL/MISSING` classifications.

### Targets and Schemas

Inspect `internal/config/targets.yaml` for names, aliases, global/project paths, and collisions, then compare configuration validation, schemas, and target tests. Never assume every target supports the same resource kinds.

### Handler Split

Find large `cmd/skillshare/*.go` files and verify that dispatch files contain only flags and mode routing while core logic, rendering, prompts, TUIs, batching, and resolution follow `cli-development` conventions. Roughly 300 lines is a review trigger, not an automatic failure.

### Oplog

Mutating commands should write the operations log; read-only commands should not write merely for consistency. Inspect success and error paths, duration, arguments, and the audit-log boundary. Determine which commands mutate state from current behavior rather than a fixed historical list.

### Web API

Compare routes in `internal/server/server.go`, `handler_*.go` files, handler tests, and `ui/src/api/client.ts`. CLI-only or API-only behavior may be intentional; inspect the product surface before reporting an issue.

`GET /api/resources` returns `resources` and `sourceLinkWarnings` (one string per first-level source link skipped during discovery). Healthy resources remain in the inventory when a link is unavailable. It also returns `linked_repos: [{name, target}]` so the Updates tab knows which followed Git checkouts are user-owned before running a check.

`GET /api/check` and the `done` event of `GET /api/check/stream` return the same `linked_repos` field separately from `tracked_repos`. Linked checkouts are excluded from Git checks and work-unit totals. Dashboard update endpoints return a `skipped` result for them before running Git, including force retries and skills within those checkouts.

Source-link endpoints (skills only, in the dashboard's current global/project mode):

| Endpoint | Request | Response |
|---|---|---|
| `POST /api/source-links` | `{path, name?, enable?}` | `{path, target, kind, warning}`; `kind` is `symlink` or `junction` |
| `DELETE /api/source-links/{name}` | First-level link name | `{success, name}`; moves only the link to trash |

Both call `internal/sourcelink`, share the uninstall routes' middleware, and return guard refusals as HTTP 400 with the core reason in the string `error` field. `enable: true` persists `follow_source_links` in the current config and refreshes the follow snapshot. The skills page's **Link folder** dialog offers that setting only while it is off. `GET /api/resources` adds optional `linkName` and `linkTarget` fields to skills beneath a followed first-level source link; the target is resolved by the same discovery policy, and skipped links are not identified as followed. Linked groups use a single linked badge and an icon-only **Unlink** action in list/cards headers; tree rows keep the target and secondary **Unlink** action in the detail pane. Linked groups have no Update repo action or group context menu; the Updates tab covers followed checkouts. Unlink confirms target preservation, affected skills on the next sync, and Trash-page recovery before sending a request; linked skill uninstall confirms that the selected skills move out of the linked folder into trash.

## Report

Use a concise table for each dimension:

| Item | Status | Evidence | Impact |
|---|---|---|---|

End with confirmed issues, intentional differences, uncertainty, and recommended priority. Do not fix findings during an audit. If the user later requests fixes, load the appropriate implementation topic.
