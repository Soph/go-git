#!/bin/bash
#
# Run git test suites against the go-git CLI and report results.
#
# Usage:
#   ./cli/git-go/run-tests.sh <git-dist-path> <cli-bin-path> [--verbose] [test-files...]
#
# Environment:
#   GIT_SKIP_TESTS  - space-separated list of tests to skip (e.g., "t0001.3 t7004.10")

set -e

GIT_DIST="$(cd "$1" && pwd)"
CLI_BIN="$(cd "$2" && pwd)"
shift 2

VERBOSE=false
if [ "$1" = "--verbose" ]; then
    VERBOSE=true
    shift
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SKIP_LIST="$SCRIPT_DIR/skip-list.txt"
UNIMPL_LOG=$(mktemp)
trap "rm -f $UNIMPL_LOG" EXIT

# Load skip list if it exists and GIT_SKIP_TESTS isn't already set.
if [ -z "$GIT_SKIP_TESTS" ] && [ -f "$SKIP_LIST" ]; then
    GIT_SKIP_TESTS=$(grep '^t' "$SKIP_LIST" | tr '\n' ' ')
fi

DEFAULT_SUITES="t0000-basic.sh t0001-init.sh t1006-cat-file.sh t1500-rev-parse.sh t3200-branch.sh t5510-fetch.sh t5516-fetch-push.sh t7004-tag.sh t7508-status.sh"
TESTS="${@:-$DEFAULT_SUITES}"

skip_count=$(echo "$GIT_SKIP_TESTS" | wc -w | tr -d ' ')
if [ "$skip_count" -gt 0 ] 2>/dev/null; then
    echo "  (skipping $skip_count known-unimplemented tests)"
fi

total_pass=0
total_skip=0
total_all=0

for t in $TESTS; do
    if $VERBOSE; then
        cd "$GIT_DIST/t" && \
            GIT_TEST_INSTALLED="$CLI_BIN" \
            GIT_TEST_UNIMPLEMENTED_LOG="$UNIMPL_LOG" \
            GIT_SKIP_TESTS="$GIT_SKIP_TESTS" \
            bash "./$t" -v 2>&1
        continue
    fi

    output=$(cd "$GIT_DIST/t" && \
        GIT_TEST_INSTALLED="$CLI_BIN" \
        GIT_TEST_UNIMPLEMENTED_LOG="$UNIMPL_LOG" \
        GIT_SKIP_TESTS="$GIT_SKIP_TESTS" \
        bash "./$t" 2>&1) || true

    pass=$(echo "$output" | grep "^ok" | grep -v "# skip" | wc -l | tr -d ' ')
    skip=$(echo "$output" | grep "^ok" | grep "# skip" | wc -l | tr -d ' ')
    count=$(echo "$output" | tail -1 | grep -o '[0-9]*$')
    fail=$((count - pass - skip))

    printf "%-30s pass %-4s skip %-4s fail %-4s / %s\n" "$t" "$pass" "$skip" "$fail" "$count"

    total_pass=$((total_pass + pass))
    total_skip=$((total_skip + skip))
    total_all=$((total_all + count))
done

if $VERBOSE; then
    exit 0
fi

total_fail=$((total_all - total_pass - total_skip))
printf "%-30s pass %-4s skip %-4s fail %-4s / %s\n" "TOTAL" "$total_pass" "$total_skip" "$total_fail" "$total_all"
echo ""

if [ -s "$UNIMPL_LOG" ]; then
    echo "Unimplemented commands hit during tests:"
    sort "$UNIMPL_LOG" | uniq -c | sort -rn
fi

if [ "$total_fail" -gt 0 ]; then
    echo ""
    echo "$total_fail failing tests are bugs in implemented commands."
    exit 1
fi
