#!/bin/bash
#
# Generate a skip list for git test suites run against the go-git CLI.
#
# Tests are skipped when they fail AND the failure output contains
# "not yet implemented" or "is not a git command" — indicating an
# unimplemented command was hit.
#
# Usage:
#   ./cli/git-go/gen-skip-list.sh <git-dist-path> <cli-bin-path> [test-files...]
#
# Output: writes cli/git-go/skip-list.txt

set -e

GIT_DIST="$(cd "$1" && pwd)"
CLI_BIN="$(cd "$2" && pwd)"
shift 2

TESTS="${@:-t0000-basic.sh t0001-init.sh t1006-cat-file.sh t1500-rev-parse.sh t3200-branch.sh t5510-fetch.sh t5516-fetch-push.sh t7004-tag.sh t7508-status.sh}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SKIP_FILE="$SCRIPT_DIR/skip-list.txt"

cat > "$SKIP_FILE" <<'EOF'
# Auto-generated skip list for go-git CLI tests.
# Tests listed here fail due to unimplemented commands/features.
# Re-generate with: make gen-skip-list
#
# Format: <test-name>.<test-number> (consumed by GIT_SKIP_TESTS)
EOF

for t in $TESTS; do
    test_base="${t%.sh}"

    # Run verbose, capture output.
    output=$(cd "$GIT_DIST/t" && \
        GIT_TEST_INSTALLED="$CLI_BIN" \
        bash "./$t" -v 2>&1) || true

    # Parse output: for each test block, check if it failed AND contains
    # an unimplemented command marker.
    skip_count=0
    current_num=""
    is_fail=false
    is_unimpl=false

    while IFS= read -r line; do
        # Detect test start: "expecting success of NNNN.N 'description':"
        if [[ "$line" =~ ^expecting\ (success|failure)\ of\ [0-9]+\.([0-9]+)\  ]]; then
            # Emit previous test if it was failing + unimplemented.
            if $is_fail && $is_unimpl && [ -n "$current_num" ]; then
                echo "$test_base.$current_num" >> "$SKIP_FILE"
            fi
            current_num="${BASH_REMATCH[2]}"
            is_fail=false
            is_unimpl=false
        fi

        if [[ "$line" =~ ^not\ ok\  ]]; then
            is_fail=true
        fi

        if [[ "$line" == *"not yet implemented in go-git"* ]] || \
           [[ "$line" == *"is not a git command"* ]]; then
            is_unimpl=true
        fi
    done <<< "$output"

    # Handle last test.
    if $is_fail && $is_unimpl && [ -n "$current_num" ]; then
        echo "$test_base.$current_num" >> "$SKIP_FILE"
    fi

    skip_count=$(grep -c "^${test_base}\." "$SKIP_FILE" 2>/dev/null || echo 0)
    fail_count=$(echo "$output" | grep "^not ok" | wc -l | tr -d ' ')
    pass_count=$(echo "$output" | grep "^ok" | wc -l | tr -d ' ')
    echo "$t: $skip_count skipped (unimplemented), $((fail_count - skip_count)) real failures, $pass_count passed"
done

total=$(grep -c '^t' "$SKIP_FILE" 2>/dev/null || echo 0)
echo ""
echo "Skip list written to $SKIP_FILE ($total tests)"
