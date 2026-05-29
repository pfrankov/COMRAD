#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

MAX_GO_SOURCE_FILE_LINES="${COMRAD_MAX_GO_SOURCE_FILE_LINES:-500}"
MAX_GO_TEST_FILE_LINES="${COMRAD_MAX_GO_TEST_FILE_LINES:-1000}"
MAX_AGENTS_LINES="${COMRAD_MAX_AGENTS_LINES:-200}"
MAX_MEMORY_LINES="${COMRAD_MAX_MEMORY_LINES:-200}"
MAX_SKILL_LINES="${COMRAD_MAX_SKILL_LINES:-1000}"
MAX_SH_FILE_LINES="${COMRAD_MAX_SH_FILE_LINES:-350}"
MAX_FUNC_LINES="${COMRAD_MAX_FUNC_LINES:-140}"
MAX_FUNC_BRANCHES="${COMRAD_MAX_FUNC_BRANCHES:-10}"

fail=0

project_files() {
  find "$ROOT" -path "$ROOT/.tools" -prune -o -path "$ROOT/.e2e" -prune -o -path "$ROOT/dist" -prune -o -path "$ROOT/.agents" -prune -o -path "$ROOT/web/dashboard/node_modules" -prune -o "$@"
}

check_file_lines_from() {
  limit="$1"
  label="$2"
  while IFS= read -r file; do
    lines="$(wc -l < "$file" | tr -d ' ')"
    if [ "$lines" -gt "$limit" ]; then
      printf '%s exceeds %s line limit (%s): %s lines\n' "$file" "$limit" "$label" "$lines" >&2
      exit 1
    fi
  done
}

check_required_file() {
  file="$1"
  limit="$2"
  if [ ! -f "$ROOT/$file" ]; then
    printf '%s is required\n' "$ROOT/$file" >&2
    return 1
  fi
  lines="$(wc -l < "$ROOT/$file" | tr -d ' ')"
  if [ "$lines" -gt "$limit" ]; then
    printf '%s exceeds %s line limit: %s lines\n' "$ROOT/$file" "$limit" "$lines" >&2
    return 1
  fi
}

project_files -name '*_test.go' -type f -print | check_file_lines_from "$MAX_GO_TEST_FILE_LINES" 'Go test file' || fail=1
project_files -name '*.go' ! -name '*_test.go' -type f -print | check_file_lines_from "$MAX_GO_SOURCE_FILE_LINES" 'Go source file' || fail=1
project_files -name '*.sh' -type f -print | check_file_lines_from "$MAX_SH_FILE_LINES" 'shell script' || fail=1
check_required_file AGENTS.md "$MAX_AGENTS_LINES" || fail=1
check_required_file MEMORY.md "$MAX_MEMORY_LINES" || fail=1
check_required_file skills/comrad/SKILL.md "$MAX_SKILL_LINES" || fail=1
if [ -f "$ROOT/SKILL.md" ]; then
  printf '%s must move to %s\n' "$ROOT/SKILL.md" "$ROOT/skills/comrad/SKILL.md" >&2
  fail=1
fi
if project_files \( -name 'README.md' -o -name 'AGENTS.md' -o -name 'MEMORY.md' -o -name 'SKILL.md' -o -path "$ROOT/docs/*" \) -type f -print | xargs grep -n 'PRD\.md\|PRD' >/tmp/comrad-prd-refs.$$ 2>/dev/null; then
  cat /tmp/comrad-prd-refs.$$ >&2
  rm -f /tmp/comrad-prd-refs.$$
  fail=1
else
  rm -f /tmp/comrad-prd-refs.$$
fi

awk -v max_lines="$MAX_FUNC_LINES" -v max_branches="$MAX_FUNC_BRANCHES" '
function reset() {
  in_func = 0
  depth = 0
  lines = 0
  branches = 0
  name = ""
}
function count_branches(line, copy) {
  copy = line
  return gsub(/(^|[^[:alnum:]_])(if|for|switch|select|case)([^[:alnum:]_]|$)/, "&", copy)
}
function count_delta(line, i, c, delta) {
  delta = 0
  for (i = 1; i <= length(line); i++) {
    c = substr(line, i, 1)
    if (c == "{") delta++
    if (c == "}") delta--
  }
  return delta
}
function finish() {
  if (!in_func) return
  if (lines > max_lines) {
    printf "%s:%d function exceeds %d line limit: %d lines (%s)\n", file, start, max_lines, lines, name > "/dev/stderr"
    bad = 1
  }
  if (branches > max_branches) {
    printf "%s:%d function exceeds cyclomatic complexity limit %d: %d branch tokens (%s)\n", file, start, max_branches, branches, name > "/dev/stderr"
    bad = 1
  }
}
FNR == 1 {
  finish()
  reset()
  file = FILENAME
}
/^func[[:space:]]/ {
  finish()
  reset()
  in_func = 1
  start = FNR
  name = $0
}
in_func {
  lines++
  branches += count_branches($0)
  depth += count_delta($0)
  if (depth == 0 && lines > 1) {
    finish()
    reset()
  }
}
END {
  finish()
  exit bad
}
' $(project_files -name '*.go' -type f -print) || fail=1

exit "$fail"
