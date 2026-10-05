---
sidebar_position: 3
---

# unlink

Remove a link created by [`skillshare link`](./link.md), or any first-level link in the skills source. Only the link moves to trash; the folder it points at is kept.

```bash
skillshare unlink _dev-skills              # Remove the link
skillshare unlink _team -p                 # Remove a project source link
```

## When to Use

- Stop using a linked checkout without touching its files
- Remove a link before relinking it under another name

```text
$ skillshare unlink _dev-skills
✓ Unlinked _dev-skills → trash, kept 7 days; its target is untouched

Next
  skillshare sync                       remove its skills from your targets
  skillshare trash restore _dev-skills  undo
```

## What Happens

`unlink` moves the link entry to trash the way [`uninstall`](./uninstall.md) removes a first-level link; it shares that code path. The trash lists it as `link → <target>`, and `skillshare trash restore <name>` recreates the link (or junction), never a copy of its target. Run `skillshare sync` afterwards to remove its skills from your targets.

It refuses a name that is not a first-level link, so it never removes a regular skill or folder:

```text
$ skillshare unlink my-skill
✗ cannot unlink my-skill: my-skill is not a link
```

| Reason | Cause |
|--------|-------|
| `<name> not found in source` | Nothing with that name is directly under the source |
| `<name> is not a link` | The entry is a regular directory or file; use [`uninstall`](./uninstall.md) |
| `"<name>" is not a first-level name` | The name contains a path separator or is `.` / `..` |

## Options

| Flag | Description |
|------|-------------|
| `--project, -p` | Use project-level config in current directory |
| `--global, -g` | Use global config (`~/.config/skillshare`) |
| `--help, -h` | Show help |

## Related

- [link](./link.md) — Create a link
- [trash](./trash.md) — Restore a removed link
- [Configuration — `follow_source_links`](../targets/configuration.md#follow_source_links) — Discovery behavior and writes through the link
