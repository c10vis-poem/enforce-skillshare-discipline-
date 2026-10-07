#!/usr/bin/env bash
# Real OpenCode skill discovery against skillshare's cross_target_discovery
# check. Used by ai_docs/tests/opencode_skill_discovery_runbook.md; run inside
# the devcontainer with a throwaway HOME.
#
#   skill-discovery.sh install           install opencode-ai into /tmp/opencode-skills
#   skill-discovery.sh fixture filtered  claude-only is excluded from opencode
#   skill-discovery.sh fixture all       every skill goes to both targets
#   skill-discovery.sh loaded [VAR=1..]  skills OpenCode loads, as JSON
#   skill-discovery.sh clean             remove the install and fixture folders
set -euo pipefail

OC_DIR=/tmp/opencode-skills
OC="$OC_DIR/node_modules/.bin/opencode"
SS=/workspace/bin/skillshare
BASE="$HOME/.config/skillshare"

skill() {
  mkdir -p "$BASE/skills/$1"
  printf -- '---\nname: %s\ndescription: %s\n---\nbody\n' "$1" "$2" >"$BASE/skills/$1/SKILL.md"
}

case "${1:-}" in
install)
  mkdir -p "$OC_DIR"
  (cd "$OC_DIR" && npm i --no-fund --no-audit opencode-ai >/dev/null 2>&1)
  printf 'opencode %s\n' "$("$OC" --version)"
  ;;
fixture)
  rm -rf "$BASE/skills" "$HOME/.claude/skills" "$HOME/.config/opencode/skills" "$HOME/.agents/skills"
  skill shared "synced to both targets"
  skill claude-only "kept out of opencode"
  exclude=""
  [ "${2:-}" = filtered ] && exclude="      exclude: [claude-only]"
  cat >"$BASE/config.yaml" <<EOF
source: $BASE/skills
mode: merge
targets:
  claude:
    skills:
      path: ~/.claude/skills
  opencode:
    skills:
      path: ~/.config/opencode/skills
$exclude
EOF
  "$SS" sync -g >/dev/null
  ;;
loaded)
  shift
  # A clean working directory keeps project skill folders out of the result.
  cd "$(mktemp -d)"
  env "$@" "$OC" debug skill | jq -c '[.[] | select(.location != "<built-in>") | {name, from: (.location | sub("^" + env.HOME + "/"; "~/") | sub("/[^/]+/SKILL.md$"; ""))}] | sort_by(.name)'
  ;;
clean)
  rm -rf "$OC_DIR" "$BASE/skills" "$HOME/.claude/skills" "$HOME/.config/opencode/skills"
  ;;
*)
  echo "usage: $0 install|fixture filtered|fixture all|loaded [VAR=1 ...]|clean" >&2
  exit 2
  ;;
esac
