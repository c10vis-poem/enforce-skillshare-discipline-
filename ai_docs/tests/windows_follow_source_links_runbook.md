# Windows Runbook: Followed Source Links (Junctions)

Manual runbook (not mdproof): it runs on a real Windows machine, driven from a macOS
host through `scripts/windows/utm.sh`. It checks that `follow_source_links: true`
discovers skills behind a junction placed directly under the skills source, that an
absent target blocks deletions instead of pruning, that writes through the junction
land in the real checkout, and that trashing the junction never touches the checkout.

## Scope

- `_dev-skills` junction (tag `0xa0000003`) to a git checkout under another folder:
  `list` shows `_dev-skills/foo` and `_dev-skills/bar`; `doctor` prints
  `followed as a directory (follow_source_links)`; `sync` creates
  `_dev-skills__foo` and `_dev-skills__bar` junctions whose targets are the logical
  `<source>\_dev-skills\<name>` paths; a second `sync` does not recreate them.
- An edit in the checkout is readable through the target junction without a sync.
- Checkout renamed away (stands in for an unmounted drive): `list` hides the group,
  `sync` prints `source link _dev-skills not followed: target is missing; kept
  existing target entries, nothing pruned this run`, the target junctions stay,
  `doctor` reports the link as not followed. Renaming it back and syncing restores
  normal output.
- `uninstall _dev-skills/bar --force` moves `bar` out of the checkout into the trash;
  `trash restore _dev-skills/bar` puts it back in the checkout.
- `uninstall _dev-skills --force` removes only the junction; the checkout keeps
  `bar, foo`; `trash list` shows `_dev-skills  link → <checkout>`; `trash restore
  _dev-skills` recreates a junction (tag `0xa0000003`).
- `follow_source_links: false`: `list` hides the group and `doctor` prints
  `not followed by discovery; its contents are invisible to skillshare. Set
  follow_source_links: true to follow it`.
- `_self` (junction to the source root) and `_tgt` (junction to the sync target)
  are always skipped with a warning naming the reason.

## Environment

Same as `windows_file_links_runbook.md`: a UTM guest with the guest agent, the
desktop user logged in, everything under `C:\Users\Public\sstest\`. The script
overrides `USERPROFILE`, `HOME`, `APPDATA`, `LOCALAPPDATA` and `TEMP`, and aborts
unless `status --json` reports the source under the test root. Git on the guest is
optional; without it the checkout gets a bare `.git\HEAD` so tracked-repo detection
still works, but git-status lines in the report differ.

## Steps

1. Build a pinned binary in the devcontainer (`amd64` for x64 guests):

   ```bash
   OUT=/tmp/skillshare-utm scripts/windows/utm.sh build <ref> arm64
   ```

2. Create the test folder and push the binary and script:

   ```bash
   printf 'New-Item -ItemType Directory -Force C:\\Users\\Public\\sstest | Out-Null; "ready"\n' > /tmp/mk.ps1
   scripts/windows/utm.sh ps /tmp/mk.ps1
   scripts/windows/utm.sh push /tmp/skillshare-utm/ss.exe 'C:\Users\Public\sstest\ss.exe'
   scripts/windows/utm.sh push scripts/windows/e2e-follow-source-links.ps1 'C:\Users\Public\sstest\e2e-follow.ps1'
   ```

3. Run as the desktop user and poll for the report:

   ```bash
   scripts/windows/utm.sh task sstest-follow 'C:\Users\Public\sstest\e2e-follow.ps1' -- \
     '-Exe C:\Users\Public\sstest\ss.exe -Root C:\Users\Public\sstest\follow -Out C:\Users\Public\sstest\follow.txt'
   scripts/windows/utm.sh pull 'C:\Users\Public\sstest\follow.txt'   # repeat until the last line is DONE
   ```

4. Clean up: `scripts/windows/utm.sh clean`.

## Pass Criteria

- Every `rc=` line in the report is `rc=0` except none; no `ABORT`.
- `sync #1` section: `_dev-skills__foo` and `_dev-skills__bar` are `LinkType=Junction`
  with `Target=<source>\_dev-skills\foo|bar` and `read=yes`.
- `foo target recreated on second sync: False`.
- `edit in the checkout` section: `read=yes: foo v2`.
- `simulate unmounted drive` section: the `nothing pruned this run` warning, both
  target junctions still present (`read=NO`), `demo-skill` still `read=yes`.
- `write through the link` section: `checkout entries: foo` after uninstall and
  `checkout entries: bar, foo` after restore.
- `uninstall the link itself` section: `_dev-skills link exists: False`, `checkout
  entries: bar, foo`, trash shows `link →`, restore yields `LinkType=Junction`.
- `follow off` section: the `Set follow_source_links: true` doctor line.

Report which token (full or basic-user) and architecture the result came from.

## Developer Mode round (symlinks, project mode)

`scripts/windows/e2e-follow-source-links-devmode.ps1` repeats the core checks with directory
symlinks (`mklink /D`) instead of junctions and a project (`.skillshare/config.yaml` with
`follow_source_links: true`), where the per-skill target links are relative symlinks. Enable
Developer Mode in the guest first, as SYSTEM through `utm.sh ps`:

```powershell
reg add HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock /v AllowDevelopmentWithoutDevLicense /t REG_DWORD /d 1 /f
```

Run it like the junction script, with `-Root C:\Users\Public\sstest\devmode` and
`-Out C:\Users\Public\sstest\devmode.txt`. The script probes with `mklink`, not
`New-Item -ItemType SymbolicLink`: Windows PowerShell 5.1 does not pass the
unprivileged-create flag and fails even with Developer Mode on.

Pass criteria:

- `sync #1`: `_dev-skills__foo`, `_dev-skills__bar`, `_vendor__baz` and `local-skill` are
  `LinkType=SymbolicLink` with relative targets `..\..\.skillshare\skills\<logical path>`
  (the followed link name stays in the link text, never the checkout path) and `read=yes`.
- `sync #2`: `foo target recreated on second sync: False` and `sync output mentions reformat: False`.
- `simulate unmounted drive`: the `nothing pruned this run` warning, the two `_dev-skills__*`
  links kept (`read=NO`), and `doctor -p` prints `2 links behind an unavailable source link,
  kept until it is back` with no `prune` suggestion.
- `global mode with Developer Mode on`: target entries are still `LinkType=Junction` with the
  absolute logical path, and the second sync does not reformat them.

## History

- 2026-10-05, ARM64 guest, full desktop-user token, head `e3c9763e1`: the first run
  on `660cc33c2` found that `filepath.EvalSymlinks` leaves a junction unresolved on
  Windows, so discovery followed the junction entry itself and found no skills;
  fixed by resolving the link text with `utils.ResolveLinkTarget` first.
- 2026-10-05, ARM64 guest, full desktop-user token, head `8092b234b`, Developer Mode on:
  the symlink round passed (12 `rc=0`); relative symlinks keep the logical tail, the
  unavailable-link doctor warning shows, global mode still uses junctions.
