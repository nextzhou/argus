#!/usr/bin/env bash

set -euo pipefail

max_bytes="${ARGUS_MAX_STAGED_FILE_BYTES:-5242880}"

is_binary_executable_mime() {
  case "$1" in
    application/octet-stream|application/x-mach-binary|application/x-executable|application/x-pie-executable)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

staged_paths=()
while IFS= read -r -d '' path; do
  staged_paths+=("$path")
done < <(git diff --cached --name-only --diff-filter=AM -z)

if [ "${#staged_paths[@]}" -eq 0 ]; then
  exit 0
fi

failures=()

for path in "${staged_paths[@]}"; do
  blob_size="$(git cat-file -s ":$path")"
  if [ "$blob_size" -gt "$max_bytes" ]; then
    failures+=("staged file exceeds ${max_bytes} bytes: $path (${blob_size} bytes)")
  fi

  file_mode="$(git ls-files --stage -- "$path" | awk '{print $1}')"
  if [ "$file_mode" != "100755" ]; then
    continue
  fi

  mime_type="$(git show ":$path" | file --brief --mime-type -)"
  if is_binary_executable_mime "$mime_type"; then
    failures+=("staged executable binary detected: $path ($mime_type)")
  fi
done

if [ "${#failures[@]}" -eq 0 ]; then
  exit 0
fi

printf '%s\n' 'pre-commit blocked staged files:'
for failure in "${failures[@]}"; do
  printf '  - %s\n' "$failure"
done
printf '%s\n' 'Build outputs should stay under ./bin/ and out of Git history.'
printf '%s\n' 'Override ARGUS_MAX_STAGED_FILE_BYTES only when the large file is intentional.'

exit 1
