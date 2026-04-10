#!/usr/bin/env bash

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

scan_with_rg() {
  git ls-files -z | xargs -0 rg -n -H --pcre2 --color=never '[\p{Han}]'
}

scan_with_perl() {
  local file
  local found=1
  local output=""

  while IFS= read -r -d '' file; do
    local file_matches
    local file_status

    set +e
    file_matches="$(perl -CSDA -ne 'if (/\p{Han}/) { chomp; print "$ARGV:$.:$_\n"; $found = 1 } END { exit($found ? 0 : 1) }' "$file" 2>&1)"
    file_status=$?
    set -e

    if [ "$file_status" -eq 0 ]; then
      output+="$file_matches"$'\n'
      found=0
      continue
    fi

    if [ "$file_status" -ne 1 ]; then
      printf '%s\n' "$file_matches"
      return "$file_status"
    fi
  done < <(git ls-files -z)

  if [ "$found" -eq 0 ]; then
    printf '%s' "$output"
  fi

  return "$found"
}

if command -v rg >/dev/null 2>&1; then
  set +e
  matches="$(scan_with_rg 2>&1)"
  status=$?
  set -e
elif command -v perl >/dev/null 2>&1; then
  set +e
  matches="$(scan_with_perl 2>&1)"
  status=$?
  set -e
else
  printf '%s\n' 'English-only check failed: neither rg nor perl is available.'
  exit 127
fi

if [ "$status" -eq 1 ]; then
  exit 0
fi

if [ "$status" -eq 0 ]; then
  printf '%s\n' 'English-only check failed: found Han characters in tracked files:'
  printf '%s\n' "$matches"
  exit 1
fi

printf '%s\n' 'English-only check failed: unable to scan tracked files.'
printf '%s\n' "$matches"
exit "$status"
