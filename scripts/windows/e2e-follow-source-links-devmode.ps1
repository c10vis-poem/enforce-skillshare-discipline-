# Windows E2E for follow_source_links with Developer Mode on: directory symlinks (not junctions) under
# the skills source, and project mode, where the per-skill target links are relative symlinks that must
# keep the logical <source>\_dev-skills\<name> tail. Runs one binary in an isolated home under -Root and
# writes a report to -Out; the last line is "DONE". See ai_docs/tests/windows_follow_source_links_runbook.md.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File e2e-follow-source-links-devmode.ps1 `
#          -Exe C:\Users\Public\sstest\ss.exe -Root C:\Users\Public\sstest\devmode -Out C:\Users\Public\sstest\devmode.txt
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
function Skill([string]$dir, [string]$name, [string]$body) {
  New-Item -ItemType Directory -Force $dir | Out-Null
  "---`nname: $name`ndescription: $name`n---`n$body`n" | Set-Content -NoNewline "$dir\SKILL.md"
}

# --- isolation ---
if (Test-Path $Root) { cmd /c "rmdir /s /q `"$Root`"" }
$H = Join-Path $Root 'home'
$env:USERPROFILE = $H; $env:HOME = $H
$env:APPDATA = "$H\AppData\Roaming"; $env:LOCALAPPDATA = "$H\AppData\Local"
$env:TEMP = "$Root\tmp"; $env:TMP = "$Root\tmp"
foreach ($v in 'XDG_CONFIG_HOME','XDG_DATA_HOME','XDG_STATE_HOME','XDG_CACHE_HOME','SKILLSHARE_CONFIG') { Remove-Item "env:$v" -ErrorAction SilentlyContinue }
$env:NO_COLOR = '1'
New-Item -ItemType Directory -Force $env:APPDATA, $env:LOCALAPPDATA, $env:TEMP, "$env:APPDATA\skillshare" | Out-Null

Section 'environment'
Log "exe=$Exe"
Log "whoami=$(whoami)"
Log "arch=$env:PROCESSOR_ARCHITECTURE os=$([Environment]::OSVersion.VersionString)"
$dm = (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock' -ErrorAction SilentlyContinue).AllowDevelopmentWithoutDevLicense
Log "AllowDevelopmentWithoutDevLicense=$dm"
# Probe with mklink, not New-Item: Windows PowerShell 5.1 does not pass the unprivileged-create flag.
New-Item -ItemType Directory -Force "$Root\probe\t" | Out-Null
cmd /c "mklink /D `"$Root\probe\l`" t" 2>&1 | ForEach-Object { Log "$_" }
Log "desktop user can create a relative directory symlink: $((Get-Item -LiteralPath "$Root\probe\l" -Force -ErrorAction SilentlyContinue).LinkType -eq 'SymbolicLink')"

# --- fixture: a project with its own skills source; a checkout outside the project; a folder inside it ---
$P = "$Root\proj"
$S = "$P\.skillshare\skills"
$T = "$P\.claude\skills"
$D = "$Root\drive\dev-skills"
Skill "$S\local-skill" 'local-skill' 'local v1'
Skill "$D\foo" 'foo' 'foo v1'
Skill "$D\bar" 'bar' 'bar v1'
Skill "$P\vendor\skills\baz" 'baz' 'baz v1'
New-Item -ItemType Directory -Force "$D\.git" | Out-Null
'ref: refs/heads/main' | Set-Content "$D\.git\HEAD"
# Directory symlinks this time (mklink /D), not junctions: Developer Mode lets the desktop user create them.
cmd /c "mklink /D `"$S\_dev-skills`" `"$D`"" 2>&1 | ForEach-Object { Log "$_" }
cmd /c "mklink /D `"$S\_vendor`" `"$P\vendor\skills`"" 2>&1 | ForEach-Object { Log "$_" }
@"
follow_source_links: true
targets:
  - name: claude
    path: .claude/skills
    mode: merge
"@ | Set-Content "$P\.skillshare\config.yaml"
'' | Set-Content "$P\.skillshare\.gitignore"
Inspect "$S\_dev-skills"
Inspect "$S\_vendor"
Set-Location $P

Section 'isolation check (status -p --json source.path)'
$st = try { (& $Exe status -p --json 2> "$env:TEMP\stderr.txt" | Out-String) | ConvertFrom-Json } catch { $null }
Log "source.path=$($st.source.path)"
if (-not $st -or -not $st.source.path -or -not $st.source.path.StartsWith($Root, 'OrdinalIgnoreCase')) { Log 'ABORT: project source is not under the test root'; Log 'DONE'; exit 1 }

function InspectAll { Inspect "$T\_dev-skills__foo"; Inspect "$T\_dev-skills__bar"; Inspect "$T\_vendor__baz"; Inspect "$T\local-skill" }

Section 'list / doctor (project, follow on)'
Run @('list', '-p', '--no-tui')
Run @('doctor', '-p')
Section 'sync #1 (project): relative symlinks with the logical tail'
Run @('sync', '-p')
InspectAll
Section 'sync #2 (idempotent, no reformat)'
$before = (Get-Item -LiteralPath "$T\_dev-skills__foo" -Force -ErrorAction SilentlyContinue).CreationTimeUtc.Ticks
Run @('sync', '-p')
$after = (Get-Item -LiteralPath "$T\_dev-skills__foo" -Force -ErrorAction SilentlyContinue).CreationTimeUtc.Ticks
Log "foo target recreated on second sync: $($before -ne $after)"
Log ("sync output mentions reformat: " + ($script:LastOut -match 'reformat'))
Run @('status', '-p')

Section 'edit in the checkout is visible through the target'
Skill "$D\foo" 'foo' 'foo v2'
Inspect "$T\_dev-skills__foo"

Section 'simulate unmounted drive: rename checkout away'
Rename-Item -LiteralPath $D "$D.off"
Run @('list', '-p', '--no-tui')
Run @('sync', '-p')
InspectAll
Run @('doctor', '-p')
Log (($script:LastOut -split "`r?`n" | Select-String 'unavailable source link|broken symlink|prune') -join "`n")

Section 'drive back'
Rename-Item -LiteralPath "$D.off" $D
Run @('sync', '-p')
Inspect "$T\_dev-skills__foo"
Run @('doctor', '-p')

Section 'global mode with Developer Mode on: junctions still used (absolute logical path)'
$G = "$H\src\skills"
Skill "$G\demo-skill" 'demo-skill' 'demo v1'
cmd /c "mklink /D `"$G\_dev-skills`" `"$D`"" 2>&1 | ForEach-Object { Log "$_" }
@"
source: '$G'
mode: merge
follow_source_links: true
targets:
  claude:
    skills:
      path: '$H\.claude\skills'
"@ | Set-Content "$env:APPDATA\skillshare\config.yaml"
Set-Location $H
Run @('sync', '-g')
Inspect "$H\.claude\skills\_dev-skills__foo"
Inspect "$H\.claude\skills\demo-skill"
Run @('sync', '-g')
Log ("sync output mentions reformat: " + ($script:LastOut -match 'reformat'))

Log ''
Log 'DONE'
