#!/usr/bin/env bash

set -euo pipefail

max_bytes="${ARGUS_MAX_STAGED_FILE_BYTES:-5242880}"
saw_build_artifact_failure=0
saw_session_guard_failure=0
saw_tmp_argus_guard_failure=0

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

is_go_test_file() {
  [[ "$1" == *_test.go ]]
}

is_high_risk_session_test_file() {
  [[ "$1" == cmd/argus/*_test.go || "$1" == tests/integration/*_test.go ]]
}

allows_tmp_argus_literal() {
  case "$1" in
    tests/integration/helpers_test.go|internal/doctor/checks_test.go)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

blob_matches() {
  local blob_content="$1"
  local pattern="$2"

  grep -Eq -- "$pattern" <<<"$blob_content"
}

has_tmp_argus_literal() {
  local blob_content="$1"

  blob_matches "$blob_content" '/tmp/argus'
}

has_hardcoded_session_literal() {
  local blob_content="$1"

  blob_matches "$blob_content" '"session_id"[[:space:]]*:[[:space:]]*"[^%"][^"]*"' ||
    blob_matches "$blob_content" '"sessionID"[[:space:]]*:[[:space:]]*"[^%"][^"]*"' ||
    blob_matches "$blob_content" '--session",[[:space:]]*"[^"]+"'
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
    saw_build_artifact_failure=1
  fi

  file_mode="$(git ls-files --stage -- "$path" | awk '{print $1}')"
  if [ "$file_mode" == "100755" ]; then
    mime_type="$(git show ":$path" | file --brief --mime-type -)"
    if is_binary_executable_mime "$mime_type"; then
      failures+=("staged executable binary detected: $path ($mime_type)")
      saw_build_artifact_failure=1
    fi
  fi

  if ! is_go_test_file "$path"; then
    continue
  fi

  blob_content="$(git show ":$path")"

  if has_tmp_argus_literal "$blob_content" && ! allows_tmp_argus_literal "$path"; then
    failures+=("test file writes /tmp/argus directly: $path (use shared session helpers)")
    saw_tmp_argus_guard_failure=1
  fi

  if is_high_risk_session_test_file "$path" && has_hardcoded_session_literal "$blob_content"; then
    failures+=("high-risk test hardcodes a session literal: $path (use session helpers)")
    saw_session_guard_failure=1
  fi
done

if [ "${#failures[@]}" -eq 0 ]; then
  exit 0
fi

printf '%s\n' 'pre-commit blocked staged files:'
for failure in "${failures[@]}"; do
  printf '  - %s\n' "$failure"
done

if [ "$saw_build_artifact_failure" -eq 1 ]; then
  printf '%s\n' 'Build outputs should stay under ./bin/ and out of Git history.'
  printf '%s\n' 'Override ARGUS_MAX_STAGED_FILE_BYTES only when the large file is intentional.'
fi

if [ "$saw_session_guard_failure" -eq 1 ]; then
  printf '%s\n' 'Use sessiontest.NewSessionID(...) in cmd tests and newDefaultSessionID(...) in integration tests.'
fi

if [ "$saw_tmp_argus_guard_failure" -eq 1 ]; then
  printf '%s\n' 'Route default /tmp/argus access through shared cleanup helpers instead of direct literals.'
fi

exit 1
