#!/usr/bin/env bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
TEST_DB_PATH="storage/test-sdk-compatibility.db"
TEST_PORT=9876
LOGNORTH_URL="http://localhost:${TEST_PORT}"
LOGNORTH_PID=""
CLEANUP_DONE=false

# Detect if SDKs are available (multi-repo setup)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HAS_TS_SDK=false
HAS_GO_SDK=false
HAS_RAILS_SDK=false

# Check for TypeScript SDK
if [ -d "$REPO_ROOT/../lognorth-sdk-ts" ]; then
    HAS_TS_SDK=true
    TS_SDK_PATH="$REPO_ROOT/../lognorth-sdk-ts"
elif [ -f "$REPO_ROOT/examples/express-basic/test.js" ]; then
    # Can still test with local example app (uses npm package)
    HAS_TS_SDK=true
    TS_SDK_PATH=""
fi

# Check for Go SDK
if [ -d "$REPO_ROOT/../lognorth-sdk-go" ]; then
    HAS_GO_SDK=true
    GO_SDK_PATH="$REPO_ROOT/../lognorth-sdk-go"
fi

# Check for Rails SDK
if [ -d "$REPO_ROOT/../lognorth-sdk-rails" ]; then
    HAS_RAILS_SDK=true
    RAILS_SDK_PATH="$REPO_ROOT/../lognorth-sdk-rails"
fi

# Cleanup function
cleanup() {
    if [ "$CLEANUP_DONE" = true ]; then
        return
    fi
    CLEANUP_DONE=true

    echo ""
    echo -e "${BLUE}🧹 Cleaning up...${NC}"

    if [ -n "$LOGNORTH_PID" ] && ps -p $LOGNORTH_PID > /dev/null 2>&1; then
        echo "  Stopping LogNorth server (PID: $LOGNORTH_PID)..."
        kill $LOGNORTH_PID 2>/dev/null || true
        wait $LOGNORTH_PID 2>/dev/null || true
    fi

    if [ -f "$TEST_DB_PATH" ]; then
        echo "  Removing test database..."
        rm -f "$TEST_DB_PATH" "${TEST_DB_PATH}-shm" "${TEST_DB_PATH}-wal"
    fi

    echo -e "${GREEN}✅ Cleanup complete${NC}"
}

# Set up trap for cleanup
trap cleanup EXIT INT TERM

echo -e "${BLUE}🧪 LogNorth SDK Compatibility Test${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Step 1: Build LogNorth
echo -e "${BLUE}📦 Building LogNorth...${NC}"
cd "$REPO_ROOT"
go build -o tmp/test-lognorth cmd/lognorth/main.go
echo -e "${GREEN}✅ Build complete${NC}"
echo ""

# Step 2: Start LogNorth server
echo -e "${BLUE}🚀 Starting LogNorth server on port ${TEST_PORT}...${NC}"
rm -f "$TEST_DB_PATH" "${TEST_DB_PATH}-shm" "${TEST_DB_PATH}-wal"

LOGNORTH_ENV=test \
LOGNORTH_DB_PATH="$TEST_DB_PATH" \
LOGNORTH_PORT="$TEST_PORT" \
./tmp/test-lognorth > ./tmp/test-server.log 2>&1 &
LOGNORTH_PID=$!

echo "  Server PID: $LOGNORTH_PID"

# Step 3: Wait for server to be ready
echo -e "${BLUE}⏳ Waiting for server to be ready...${NC}"
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    if curl -s "${LOGNORTH_URL}/_health" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Server is ready${NC}"
        break
    fi
    attempt=$((attempt + 1))
    if [ $attempt -eq $max_attempts ]; then
        echo -e "${RED}❌ Server failed to start within 30 seconds${NC}"
        echo ""
        echo "Server logs:"
        cat ./tmp/test-server.log
        exit 1
    fi
    sleep 1
done
echo ""

# Step 4: Create API key
echo -e "${BLUE}🔑 Creating test API key...${NC}"
API_KEY=$(LOGNORTH_ENV=test LOGNORTH_DB_PATH="$TEST_DB_PATH" go run cmd/create-test-key/main.go 2>&1 | tail -1)

if [ -z "$API_KEY" ]; then
    echo -e "${RED}❌ Failed to create API key${NC}"
    exit 1
fi

echo "  API Key: ${API_KEY:0:15}..."
echo -e "${GREEN}✅ API key created${NC}"
echo ""

# Export for tests
export LOGNORTH_URL
export LOGNORTH_API_KEY="$API_KEY"

TS_EXIT=0
GO_EXIT=0
RAILS_EXIT=0

# Step 5: Run TypeScript SDK tests (if available)
if [ "$HAS_TS_SDK" = true ]; then
    echo -e "${BLUE}📝 Running TypeScript SDK compatibility test...${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

    # Build and link SDK (if in multi-repo setup)
    if [ -n "$TS_SDK_PATH" ]; then
        cd "$TS_SDK_PATH"
        if [ ! -d "node_modules" ]; then
            echo "  Installing SDK dependencies..."
            npm install --silent
        fi
        echo "  Building SDK..."
        npm run build --silent
        echo "  Linking SDK..."
        npm link --silent 2>/dev/null || true
    fi

    # Install and link in example app
    cd "$REPO_ROOT/examples/express-basic"
    if [ ! -d "node_modules" ]; then
        echo "  Installing example app dependencies..."
        npm install --silent
    fi
    if [ -n "$TS_SDK_PATH" ]; then
        echo "  Linking SDK to example app..."
        npm link lognorth --silent 2>/dev/null || true
    fi

    echo "  Running test..."
    node test.js
    TS_EXIT=$?
    cd "$REPO_ROOT"
    echo ""
else
    echo -e "${YELLOW}⚠️  TypeScript SDK not found, skipping${NC}"
    echo ""
fi

# Step 6: Run Go SDK tests (if available)
if [ "$HAS_GO_SDK" = true ]; then
    echo -e "${BLUE}📝 Running Go SDK compatibility test...${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    cd "$GO_SDK_PATH"
    go test -v -run TestSDKCompatibility
    GO_EXIT=$?
    cd "$REPO_ROOT"
    echo ""
else
    echo -e "${YELLOW}⚠️  Go SDK not found, skipping${NC}"
    echo ""
fi

# Step 7: Run Rails SDK tests (if available)
if [ "$HAS_RAILS_SDK" = true ]; then
    echo -e "${BLUE}📝 Running Rails SDK compatibility test...${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    cd "$RAILS_SDK_PATH"
    if [ ! -f "Gemfile.lock" ]; then
        echo "  Installing SDK dependencies..."
        bundle install --quiet
    fi
    echo "  Running tests..."
    # Try different test patterns - Rails SDK might use minitest or rspec
    if [ -f "test/compatibility_test.rb" ]; then
        bundle exec rake test TEST=test/compatibility_test.rb
        RAILS_EXIT=$?
    elif [ -f "spec/compatibility_spec.rb" ]; then
        bundle exec rspec spec/compatibility_spec.rb
        RAILS_EXIT=$?
    elif [ -d "test" ] && ls test/*_test.rb 1>/dev/null 2>&1; then
        bundle exec rake test
        RAILS_EXIT=$?
    elif [ -d "spec" ] && ls spec/*_spec.rb 1>/dev/null 2>&1; then
        bundle exec rspec
        RAILS_EXIT=$?
    else
        echo -e "${YELLOW}  No test files found, running basic require check...${NC}"
        bundle exec ruby -e "require 'lognorth'; puts '✅ Gem loads successfully'"
        RAILS_EXIT=$?
    fi
    cd "$REPO_ROOT"
    echo ""
else
    echo -e "${YELLOW}⚠️  Rails SDK not found, skipping${NC}"
    echo ""
fi

# Summary
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}📊 Test Summary${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

if [ "$HAS_TS_SDK" = true ]; then
    if [ $TS_EXIT -eq 0 ]; then
        echo -e "${GREEN}✅ TypeScript SDK: PASSED${NC}"
    else
        echo -e "${RED}❌ TypeScript SDK: FAILED${NC}"
    fi
fi

if [ "$HAS_GO_SDK" = true ]; then
    if [ $GO_EXIT -eq 0 ]; then
        echo -e "${GREEN}✅ Go SDK: PASSED${NC}"
    else
        echo -e "${RED}❌ Go SDK: FAILED${NC}"
    fi
fi

if [ "$HAS_RAILS_SDK" = true ]; then
    if [ $RAILS_EXIT -eq 0 ]; then
        echo -e "${GREEN}✅ Rails SDK: PASSED${NC}"
    else
        echo -e "${RED}❌ Rails SDK: FAILED${NC}"
    fi
fi

echo ""

if [ $TS_EXIT -eq 0 ] && [ $GO_EXIT -eq 0 ] && [ $RAILS_EXIT -eq 0 ]; then
    echo -e "${GREEN}🎉 All SDK compatibility tests passed!${NC}"
    exit 0
else
    echo -e "${RED}❌ Some tests failed${NC}"
    exit 1
fi
