#!/bin/bash

# Test suite for install.sh
# Run: ./scripts/test_install.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SCRIPT="$SCRIPT_DIR/../install.sh"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

TESTS_RUN=0
TESTS_PASSED=0

assert_eq() {
    local expected="$1"
    local actual="$2"
    local msg="$3"
    TESTS_RUN=$((TESTS_RUN + 1))
    if [ "$expected" = "$actual" ]; then
        echo -e "${GREEN}✓${NC} $msg"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗${NC} $msg"
        echo "  Expected: $expected"
        echo "  Got:      $actual"
    fi
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local msg="$3"
    TESTS_RUN=$((TESTS_RUN + 1))
    if echo "$haystack" | grep -q "$needle"; then
        echo -e "${GREEN}✓${NC} $msg"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗${NC} $msg"
        echo "  Expected to contain: $needle"
        echo "  Got: $haystack"
    fi
}

echo "Testing install.sh"
echo "=================="
echo ""

# Test 1: Script exists and is executable
if [ -f "$INSTALL_SCRIPT" ]; then
    assert_eq "1" "1" "install.sh exists"
else
    assert_eq "1" "0" "install.sh exists"
fi

# Test 2: Script has valid bash syntax
SYNTAX_CHECK=$(bash -n "$INSTALL_SCRIPT" 2>&1 || echo "SYNTAX_ERROR")
if [ "$SYNTAX_CHECK" = "" ]; then
    assert_eq "1" "1" "install.sh has valid bash syntax"
else
    assert_eq "1" "0" "install.sh has valid bash syntax (error: $SYNTAX_CHECK)"
fi

# Test 3: Script contains required variables
SCRIPT_CONTENT=$(cat "$INSTALL_SCRIPT")
assert_contains "$SCRIPT_CONTENT" "GITHUB_REPO=\"karloscodes/lognorth\"" "Contains correct GitHub repo"
assert_contains "$SCRIPT_CONTENT" "BINARY_NAME=\"lognorth\"" "Contains correct binary name"
assert_contains "$SCRIPT_CONTENT" "lognorth.com/install" "Contains correct install URL in header"

# Test 4: Script handles architectures
assert_contains "$SCRIPT_CONTENT" "x86_64" "Handles x86_64 architecture"
assert_contains "$SCRIPT_CONTENT" "aarch64|arm64" "Handles arm64 architecture"

# Test 5: Script requires root
assert_contains "$SCRIPT_CONTENT" "id -u" "Checks for root privileges"

# Test 6: Script verifies checksums
assert_contains "$SCRIPT_CONTENT" "sha256sum" "Verifies SHA256 checksums"
assert_contains "$SCRIPT_CONTENT" "checksums.txt" "Downloads checksums file"

# Test 7: Script handles piped execution
assert_contains "$SCRIPT_CONTENT" "! -t 0" "Detects piped execution"
assert_contains "$SCRIPT_CONTENT" "/dev/tty" "Handles TTY for interactive mode"

# Test 8: Script is Linux-only
assert_contains "$SCRIPT_CONTENT" "linux" "Checks for Linux OS"

echo ""
echo "=================="
echo "Results: $TESTS_PASSED/$TESTS_RUN tests passed"

if [ "$TESTS_PASSED" -eq "$TESTS_RUN" ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
