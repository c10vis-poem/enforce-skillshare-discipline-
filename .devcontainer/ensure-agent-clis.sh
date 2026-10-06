#!/usr/bin/env bash
# Ensure the Agent CLIs that plugin work drives (claude, codex, pi) exist in the
# devcontainer, so installs can be verified against the real clients.
# Packages live on the dev-home volume and survive image rebuilds; only the
# /usr/local/bin links are container-local, so they are re-made every start.
set -euo pipefail

PREFIX="$HOME/.local/agent-clis"
# bin → npm package, optionally @version. A pinned package is reinstalled when the
# volume holds another version; an unpinned one is installed once and left alone.
declare -A PACKAGES=(
  [claude]="@anthropic-ai/claude-code"
  [codex]="@openai/codex"
  [pi]="@earendil-works/pi-coding-agent@1.0.4"
)

for bin in "${!PACKAGES[@]}"; do
  spec="${PACKAGES[$bin]}"
  name="$spec" want=""
  if [[ "${spec:1}" == *@* ]]; then name="${spec%@*}" want="${spec##*@}"; fi
  # Check the package, not the binary: a package that moved to a new name (Pi, from
  # @mariozechner) leaves the old one's binary behind, which would never be replaced.
  have=$(node -p "require('$PREFIX/lib/node_modules/$name/package.json').version" 2>/dev/null || true)
  if [ -z "$have" ] || { [ -n "$want" ] && [ "$have" != "$want" ]; }; then
    echo "▸ Installing $bin ($spec) …"
    rm -f "$PREFIX/bin/$bin"
    if ! npm install -g --prefix "$PREFIX" --no-fund --no-audit "$spec" >/dev/null 2>&1; then
      echo "⚠ Could not install $bin; plugin checks against it will report it missing." >&2
      continue
    fi
  fi
  ln -sf "$PREFIX/bin/$bin" "/usr/local/bin/$bin"
done
