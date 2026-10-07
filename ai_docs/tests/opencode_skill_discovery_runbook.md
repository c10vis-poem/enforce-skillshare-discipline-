# CLI E2E Runbook: OpenCode Skill Discovery vs Doctor

## Scope

Check `skillshare doctor`'s `cross_target_discovery` result against what a real
OpenCode loads when `claude` and `opencode` are both targets (#458). OpenCode
also reads `~/.claude/skills` and `~/.agents/skills`, and keeps one skill per
name across them.

- A skill excluded from `opencode` still reaches OpenCode through
  `~/.claude/skills`, and doctor names that skill.
- `OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1` stops OpenCode reading
  `~/.claude/skills`; with the variable set, doctor passes and notes the skip.
- A skill synced to both folders loads once, and doctor passes.

`opencode debug skill` lists loaded skills without a login or model call.
Which copy of a duplicated name wins is not fixed, so the steps check counts,
not the winning folder.

## Environment

Run inside the devcontainer with network access (step 1 installs `opencode-ai`
from npm into `/tmp/opencode-skills`). Helper: `scripts/opencode/skill-discovery.sh`.

Run mdproof directly, not inside `ssenv enter`: mdproof gives the runbook its
own HOME, while ssenv's `XDG_CONFIG_HOME` would still point skillshare and
OpenCode at the ssenv folder.

```sh
CONTAINER=$(docker compose -f .devcontainer/docker-compose.yml ps -q skillshare-devcontainer)
docker exec "$CONTAINER" /workspace/.devcontainer/ensure-mdproof.sh
docker exec "$CONTAINER" \
  mdproof --report json /workspace/ai_docs/tests/opencode_skill_discovery_runbook.md
```

## Steps

### Step 1: Build and install OpenCode

```bash
cd /workspace && make build >/dev/null
bash /workspace/scripts/opencode/skill-discovery.sh install
```

Expected:
- exit_code: 0
- regex: opencode \d+\.

### Step 2: OpenCode loads a skill excluded from it through ~/.claude/skills

```bash
S=/workspace/scripts/opencode/skill-discovery.sh
bash "$S" fixture filtered
bash "$S" loaded
```

Expected:
- exit_code: 0
- jq: map(select(.name == "claude-only")) == [{"name": "claude-only", "from": "~/.claude/skills"}]
- jq: map(select(.name == "shared")) | length == 1

### Step 3: Doctor names the leaked skill and the variable

```bash
bash /workspace/scripts/opencode/skill-discovery.sh fixture filtered
ss doctor -g --json | jq '.checks[] | select(.name == "cross_target_discovery")'
```

Expected:
- exit_code: 0
- jq: .status == "warning"
- jq: .details | any(test("loads skills missing from its own folder: claude-only$"))
- jq: .suggestions | any(test("OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1"))

### Step 4: The variable hides the skill from OpenCode and passes doctor

```bash
S=/workspace/scripts/opencode/skill-discovery.sh
bash "$S" fixture filtered
L=$(bash "$S" loaded OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1)
D=$(OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 ss doctor -g --json | jq '.checks[] | select(.name == "cross_target_discovery")')
jq -n --argjson loaded "$L" --argjson doctor "$D" '{loaded: $loaded, doctor: $doctor}'
```

Expected:
- exit_code: 0
- jq: .loaded | map(.name) == ["shared"]
- jq: .doctor.status == "pass"
- jq: .doctor.details | any(test("opencode skips .*\\.claude/skills: OPENCODE_DISABLE_CLAUDE_CODE_SKILLS is set"))

### Step 5: Skills synced to both folders load once and pass doctor

```bash
S=/workspace/scripts/opencode/skill-discovery.sh
bash "$S" fixture all
L=$(bash "$S" loaded)
D=$(ss doctor -g --json | jq '.checks[] | select(.name == "cross_target_discovery")')
jq -n --argjson loaded "$L" --argjson doctor "$D" '{loaded: $loaded, doctor: $doctor}'
```

Expected:
- exit_code: 0
- jq: .loaded | map(.name) == ["claude-only", "shared"]
- jq: .doctor.status == "pass"

### Step 6: A skill the user keeps in ~/.claude/skills reaches OpenCode and doctor names it

```bash
S=/workspace/scripts/opencode/skill-discovery.sh
bash "$S" fixture all
mkdir -p ~/.claude/skills/my-local
printf -- '---\nname: my-local\ndescription: kept by hand in claude\n---\nbody\n' > ~/.claude/skills/my-local/SKILL.md
L=$(bash "$S" loaded)
D=$(ss doctor -g --json | jq '.checks[] | select(.name == "cross_target_discovery")')
jq -n --argjson loaded "$L" --argjson doctor "$D" '{loaded: $loaded, doctor: $doctor}'
```

Expected:
- exit_code: 0
- jq: .loaded | map(select(.name == "my-local")) == [{"name": "my-local", "from": "~/.claude/skills"}]
- jq: .doctor.status == "warning"
- jq: .doctor.details | any(test("loads skills missing from its own folder: my-local$"))

### Step 7: Clean up

```bash
bash /workspace/scripts/opencode/skill-discovery.sh clean
test ! -e /tmp/opencode-skills && echo "opencode removed"
```

Expected:
- exit_code: 0
- opencode removed

## Pass Criteria

Steps 1 to 7 pass: doctor warns exactly when OpenCode loads a skill sync keeps
out of it, and passes when the variable is set or every skill reaches both
folders.
