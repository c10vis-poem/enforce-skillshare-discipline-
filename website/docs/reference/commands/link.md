---
sidebar_position: 3
---

# link

Link a folder, such as your own skills checkout, directly under the skills source. With [`follow_source_links`](../targets/configuration.md#follow_source_links) on, skillshare lists and syncs the skills inside it.

```bash
skillshare link ~/code/dev-skills                  # Link as _dev-skills
skillshare link ~/code/dev-skills --enable         # Link and turn on following
skillshare link ~/code/dev-skills --name _mine     # Choose the link name
skillshare link ../team --name _team -p            # Link into the project source
skillshare link -- -checkout                       # Link a path starting with -
```

## When to Use

- Keep editing a skills checkout where it already lives instead of installing a copy
- Use skills from an [external drive](../targets/configuration.md#skills-on-an-external-drive)
- Create the link on Windows without choosing between a junction and a symlink yourself

```text
$ skillshare link ~/code/dev-skills --enable
✓ Linked _dev-skills → ~/code/dev-skills · symlink
✓ Set follow_source_links: true ~/.config/skillshare/config.yaml

Next
  skillshare sync  link its skills into your targets
```

## What Happens

1. The link name defaults to `_` plus the folder name (`_dev-skills`). It must be a single name directly under the source.
2. The folder is checked the same way discovery checks a followed link. The link is refused when the name already exists, or when the folder is the source or a parent of it, sits inside the source, overlaps a sync target, is missing, is unreadable, or is not a directory.
3. The link is created: an absolute symlink on macOS and Linux. On Windows a junction is created, which needs no Developer Mode or elevation; a directory symlink is tried only when the junction fails.
4. When the folder has no `.git` entry, a warning says so: the link still works, but `skillshare update` cannot pull it.
5. When `follow_source_links` is off, the config is not changed unless you pass `--enable`. Without it, the command prints the same hint as `skillshare doctor`. If `--enable` cannot save the config, the new link is removed again and the command fails, so no unfollowed link is left behind. The dashboard's **Link folder** checkbox works the same way.

## Options

| Flag | Description |
|------|-------------|
| `--name <name>` | Link name in the source (default: `_<folder name>`) |
| `--enable` | Also set `follow_source_links: true` |
| `--project, -p` | Use project-level config in current directory |
| `--global, -g` | Use global config (`~/.config/skillshare`) |
| `--` | End options; allow a path starting with `-` |
| `--help, -h` | Show help |

## Without `--enable`

```text
$ skillshare link ~/code/dev-skills
✓ Linked _dev-skills → ~/code/dev-skills · symlink
! target is not a git checkout; skillshare update cannot pull it
  _dev-skills: not followed by discovery; its contents are invisible to skillshare. Set follow_source_links: true to follow it
```

The link exists, but discovery ignores it until `follow_source_links: true` is set in the config.

## Refused Links

The command exits non-zero and creates nothing:

```text
$ skillshare link ~/.claude
✗ cannot link /home/me/.claude: target overlaps sync target /home/me/.claude/skills
```

| Reason | Cause |
|--------|-------|
| `target is the source or a parent of it` | Following it would loop back into the source |
| `target is inside the source` | Its skills are already in the source |
| `target overlaps sync target <path>` | The folder is, contains, or sits inside an active skills target |
| `target is a link chain that could not be resolved` | The path is a link whose target cannot be resolved |
| `target is missing` / `target is not readable` / `target is unavailable: <error>` | The folder does not exist or cannot be read |
| `target is not a directory` | The path is a file |
| `<name> already exists` | Something with that name is already in the source |
| `"<name>" is not a first-level name` | `--name` contains a path separator or is `.` / `..` |

## Manual Links

`skillshare link` creates the same entry as a link you make yourself (`ln -s`, or `mklink /J` on Windows); discovery treats both alike. The command adds the checks above before anything is created.

## Related

- [unlink](./unlink.md) — Remove a link
- [Configuration — `follow_source_links`](../targets/configuration.md#follow_source_links) — Discovery behavior, safety guards, and writes through the link
- [doctor](./doctor.md) — Report the state of each source link
