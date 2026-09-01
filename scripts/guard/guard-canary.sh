#!/bin/bash
# scripts/guard/guard-canary.sh
#
# Proves scripts/guard/pre-commit actually fires, instead of trusting that it
# does. This is the other half of "Deterministic Guardrails & AI Governance"
# (see README): a boundary nobody verifies is exactly how BlueMonitor's
# deny-hook sat silently broken for a month before watch.ai's canary caught
# it — the hook pointed at a missing settings file and failed with no error,
# so nothing ever told anyone it had stopped enforcing.
#
# What this checks, against a disposable scratch repo (no Docker required):
#   1. A commit that touches a protected path (.github/workflows/*) is
#      REJECTED.
#   2. A commit that touches the guard's own file (scripts/guard/*) is
#      REJECTED — the guard must protect itself.
#   3. A commit that touches an ordinary source file is ALLOWED — the guard
#      must not be so broad it breaks the feature it's supposed to enable.
#   4. The guard's own log shows both a BLOCKED and an ALLOWED line — the
#      exact "did the guard log grow?" signature watch.ai's canary checks,
#      because a hook that silently no-ops leaves no trace otherwise.
#   5. (Optional, only if the opencode-selfhealing image has been built)
#      the guard is actually present and executable INSIDE the image, at
#      the path outside /workspace that scripts/analyze.sh points
#      core.hooksPath at.
#
# Usage: scripts/guard/guard-canary.sh   (or: make guard-canary)
# Exit 0 only if every check passes.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOOK_SRC="$SCRIPT_DIR/pre-commit"

PASS=0
FAIL=0

ok() {
    echo "  PASS: $1"
    PASS=$((PASS + 1))
}
bad() {
    echo "  FAIL: $1"
    FAIL=$((FAIL + 1))
}

if [ ! -x "$HOOK_SRC" ]; then
    bad "scripts/guard/pre-commit is missing or not executable at $HOOK_SRC"
    echo ""
    echo "1 check run, 0 passed, 1 failed."
    exit 1
fi

WORKDIR=$(mktemp -d)
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

REPO="$WORKDIR/repo"
HOOKS_DIR="$WORKDIR/hooks"
export GUARD_LOG="$WORKDIR/guard.log"

mkdir -p "$REPO" "$HOOKS_DIR"
cp "$HOOK_SRC" "$HOOKS_DIR/pre-commit"
chmod +x "$HOOKS_DIR/pre-commit"

git -C "$REPO" init -q -b main
git -C "$REPO" config user.email "canary@example.invalid"
git -C "$REPO" config user.name "Guard Canary"
git -C "$REPO" config core.hooksPath "$HOOKS_DIR"

attempt_commit() {
    # attempt_commit <relative-path> <content>
    local rel="$1" content="$2"
    mkdir -p "$REPO/$(dirname "$rel")"
    printf '%s\n' "$content" >"$REPO/$rel"
    git -C "$REPO" add "$rel"
    if GUARD_LOG="$GUARD_LOG" git -C "$REPO" commit -q -m "canary: $rel" >/dev/null 2>&1; then
        echo "committed"
    else
        git -C "$REPO" reset -q HEAD -- "$rel" >/dev/null 2>&1 || true
        echo "blocked"
    fi
}

echo "=== Guard Canary — live-fire test ==="
echo ""

echo "1. Protected path (.github/workflows/evil.yml) must be blocked"
result=$(attempt_commit ".github/workflows/evil.yml" "on: push")
if [ "$result" = "blocked" ]; then
    ok "CI workflow write was rejected"
else
    bad "CI workflow write was ALLOWED — the guard did not fire"
fi

echo "2. The guard's own file (scripts/guard/pre-commit) must be blocked"
result=$(attempt_commit "scripts/guard/pre-commit" "#!/bin/bash
exit 0")
if [ "$result" = "blocked" ]; then
    ok "Attempt to overwrite the guard itself was rejected"
else
    bad "The guard did not protect itself — this is the single most important check, and it failed"
fi

echo "3. An ordinary source file must be allowed"
result=$(attempt_commit "internal/example/widget.go" "package example")
if [ "$result" = "committed" ]; then
    ok "Legitimate application change was allowed"
else
    bad "Legitimate change was blocked — the guard is over-broad and would break the feature it's supposed to enable"
fi

echo "4. Guard log must show both a BLOCKED and an ALLOWED line"
if [ -f "$GUARD_LOG" ] && grep -q "BLOCKED" "$GUARD_LOG" && grep -q "ALLOWED" "$GUARD_LOG"; then
    ok "Guard log grew as expected (proof the hook actually ran, not just that git said so)"
else
    bad "Guard log missing or incomplete at $GUARD_LOG — a silent no-op hook would look exactly like this"
fi

echo "5. Optional: guard present inside the built image"
IMAGE_HOOK="/etc/opencode-guard/hooks/pre-commit"
if command -v docker >/dev/null 2>&1 && docker image inspect opencode-selfhealing:latest >/dev/null 2>&1; then
    if docker run --rm --entrypoint /bin/sh opencode-selfhealing:latest -c "test -x $IMAGE_HOOK"; then
        ok "$IMAGE_HOOK is present and executable inside opencode-selfhealing:latest"
    else
        bad "$IMAGE_HOOK is missing or not executable inside the built image"
    fi
else
    echo "  SKIP: opencode-selfhealing:latest not built here (run 'make opencode-build' first for full coverage)"
fi

echo ""
echo "$((PASS + FAIL)) check(s) run, $PASS passed, $FAIL failed."

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
