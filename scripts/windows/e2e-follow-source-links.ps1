# Windows E2E for follow_source_links: a junction directly under the skills source that points at a
# git checkout elsewhere (another folder or drive). Runs one binary in an isolated home under -Root
# and writes a report to -Out; the last line is "DONE". See ai_docs/tests/windows_follow_source_links_runbook.md.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File e2e-follow-source-links.ps1 `
#          -Exe C:\Users\Public\sstest\ss.exe -Root C:\Users\Public\sstest\follow -Out C:\Users\Public\sstest\follow.txt
param(
  [Parameter(Mandatory)] [string]$Exe,
  [Parameter(Mandatory)] [string]$Root,
  [Parameter(Mandatory)] [string]$Out
)
$ErrorActionPreference = 'Continue'
try { [Console]::OutputEncoding = [Text.Encoding]::UTF8 } catch {}
$OutputEncoding = [Text.Encoding]::UTF8

function Log([string]$s) { $s | Out-File -Encoding utf8 -Append $Out }
function Section([string]$s) { Log ""; Log "=== $s ===" }
function Run([string[]]$CliArgs) {
  Log "> skillshare $($CliArgs -join ' ')"
  $res = & $Exe @CliArgs 2>&1 | ForEach-Object { "$_" }
  $script:LastOut = ($res | Out-String)
  Log $script:LastOut.TrimEnd()
  Log "rc=$LASTEXITCODE"
}
function Inspect([string]$p) {
  $i = Get-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue
  if (-not $i) { Log "$p : MISSING"; return }
  $fs = (cmd /c "fsutil reparsepoint query `"$p`" 2>&1") | Out-String
  $tag = if ($fs -match '0x[0-9a-fA-F]{8}') { $Matches[0] } else { 'none' }
  $read = 'n/a'
  $md = Join-Path $p 'SKILL.md'
  if (Test-Path -LiteralPath $md) {
    try { $c = Get-Content -LiteralPath $md -Raw -ErrorAction Stop; $read = "yes: " + (($c -split "`r?`n")[-2]) }
    catch { $read = "NO: " + $_.Exception.Message }
  } elseif ($i.LinkType) { $read = 'NO: SKILL.md unreadable' }
  Log ("{0}`n  LinkType={1} Target={2} reparseTag={3} read={4}" -f $p, $i.LinkType, ($i.Target -join ','), $tag, $read)
}

# --- isolation ---
if (Test-Path $Root) { cmd /c "rmdir /s /q `"$Root`"" }
$H = Join-Path $Root 'home'
$env:USERPROFILE = $H; $env:HOME = $H
$env:APPDATA = "$H\AppData\Roaming"; $env:LOCALAPPDATA = "$H\AppData\Local"
$env:TEMP = "$Root\tmp"; $env:TMP = "$Root\tmp"
foreach ($v in 'XDG_CONFIG_HOME','XDG_DATA_HOME','XDG_STATE_HOME','XDG_CACHE_HOME','SKILLSHARE_CONFIG') { Remove-Item "env:$v" -ErrorAction SilentlyContinue }
$env:NO_COLOR = '1'
New-Item -ItemType Directory -Force $env:APPDATA, $env:LOCALAPPDATA, $env:TEMP | Out-Null
Set-Location $H

Section 'environment'
Log "exe=$Exe"
Log "whoami=$(whoami)"
Log "arch=$env:PROCESSOR_ARCHITECTURE os=$([Environment]::OSVersion.VersionString)"
Log "git=$(try { (git --version 2>&1) } catch { 'absent' })"

# --- fixture: source with a local skill; a "checkout" under a separate drive-like folder ---
$S = "$H\src\skills"
$D = "$Root\drive\dev-skills"
New-Item -ItemType Directory -Force "$S\demo-skill", "$D\foo", "$D\bar", "$H\.claude", "$env:APPDATA\skillshare" | Out-Null
"---`nname: demo-skill`ndescription: Demo skill`n---`nbody`n" | Set-Content -NoNewline "$S\demo-skill\SKILL.md"
"---`nname: foo`ndescription: Foo on drive`n---`nfoo v1`n" | Set-Content -NoNewline "$D\foo\SKILL.md"
"---`nname: bar`ndescription: Bar on drive`n---`nbar v1`n" | Set-Content -NoNewline "$D\bar\SKILL.md"
if (Get-Command git -ErrorAction SilentlyContinue) {
  git -C $D init -q 2>&1 | Out-Null
  git -C $D -c user.email=e2e@test -c user.name=e2e add -A 2>&1 | Out-Null
  git -C $D -c user.email=e2e@test -c user.name=e2e commit -qm init 2>&1 | Out-Null
} else {
  New-Item -ItemType Directory -Force "$D\.git" | Out-Null
  'ref: refs/heads/main' | Set-Content "$D\.git\HEAD"
}
cmd /c "mklink /J `"$S\_dev-skills`" `"$D`"" 2>&1 | ForEach-Object { Log "$_" }
cmd /c "mklink /J `"$S\_self`" `"$S`"" 2>&1 | ForEach-Object { Log "$_" }
cmd /c "mklink /J `"$S\_tgt`" `"$H\.claude\skills`"" 2>&1 | ForEach-Object { Log "$_" }
@"
source: '$S'
mode: merge
follow_source_links: true
targets:
  claude:
    skills:
      path: '$H\.claude\skills'
"@ | Set-Content "$env:APPDATA\skillshare\config.yaml"
Inspect "$S\_dev-skills"

Section 'isolation check (status --json source.path)'
$st = try { (& $Exe status --json 2> "$env:TEMP\stderr.txt" | Out-String) | ConvertFrom-Json } catch { $null }
Log "source.path=$($st.source.path)"
if (-not $st -or ($st.source.path.TrimEnd('\') -ine $S)) { Log 'ABORT: config is not under the test root'; Log 'DONE'; exit 1 }

$T = "$H\.claude\skills"
function InspectAll { Inspect "$T\_dev-skills__foo"; Inspect "$T\_dev-skills__bar"; Inspect "$T\demo-skill" }

Section 'list / doctor (follow on)'
Run @('list', '--no-tui')
Run @('doctor')
Section 'sync #1'
Run @('sync')
InspectAll
Section 'sync #2 (idempotent)'
$before = (Get-Item -LiteralPath "$T\_dev-skills__foo" -Force -ErrorAction SilentlyContinue).CreationTimeUtc.Ticks
Run @('sync')
$after = (Get-Item -LiteralPath "$T\_dev-skills__foo" -Force -ErrorAction SilentlyContinue).CreationTimeUtc.Ticks
Log "foo target recreated on second sync: $($before -ne $after)"
Run @('status')

Section 'edit in the checkout is visible through the target'
"---`nname: foo`ndescription: Foo on drive`n---`nfoo v2`n" | Set-Content -NoNewline "$D\foo\SKILL.md"
Inspect "$T\_dev-skills__foo"

Section 'simulate unmounted drive: rename checkout away'
Rename-Item -LiteralPath $D "$D.off"
Inspect "$S\_dev-skills"
Run @('list', '--no-tui')
Run @('sync')
InspectAll
Run @('status')
Run @('doctor')

Section 'drive back: rename checkout back, sync'
Rename-Item -LiteralPath "$D.off" $D
Run @('sync')
Inspect "$T\_dev-skills__foo"

Section 'write through the link: uninstall _dev-skills/bar -> trash, restore'
Run @('uninstall', '_dev-skills/bar', '--force')
Log ("checkout entries: " + ((Get-ChildItem -LiteralPath $D -Name) -join ', '))
Run @('trash', 'list')
Run @('trash', 'restore', '_dev-skills/bar')
Log ("checkout entries: " + ((Get-ChildItem -LiteralPath $D -Name) -join ', '))

Section 'uninstall the link itself: checkout untouched'
Run @('uninstall', '_dev-skills', '--force')
Log ("_dev-skills link exists: " + (Test-Path -LiteralPath "$S\_dev-skills"))
Log ("checkout entries: " + ((Get-ChildItem -LiteralPath $D -Name) -join ', '))
Run @('trash', 'list')
Run @('trash', 'restore', '_dev-skills')
Inspect "$S\_dev-skills"
Run @('sync')
Inspect "$T\_dev-skills__foo"

Section 'follow off: links invisible'
(Get-Content "$env:APPDATA\skillshare\config.yaml") -replace 'follow_source_links: true', 'follow_source_links: false' | Set-Content "$env:APPDATA\skillshare\config.yaml"
Run @('list', '--no-tui')
Run @('doctor')
Log (($script:LastOut -split "`r?`n" | Select-String 'Source link') -join "`n")

Log ''
Log 'DONE'
