#!/usr/bin/env bash

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

set +e
matches="$(git ls-files -z | xargs -0 rg -n -H --pcre2 --color=never '[\p{Han}]' 2>&1)"
status=$?
set -e

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
