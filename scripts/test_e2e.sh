#!/usr/bin/env bash
# ==============================================================================
# Flagura Automated End-to-End (E2E) Test Suite
# Tests: Server Bootstrap -> Signup -> Login -> API Key -> Flags CRUD ->
#        Go SDK + OpenFeature -> TypeScript SDK + OpenFeature ->
#        Python SDK + OpenFeature -> Rust Contract -> Live Toggling -> Governance
# ==============================================================================
set -euo pipefail

BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "\n${BOLD}${BLUE}========================================================================${NC}"
echo -e "${BOLD}${BLUE}🚀 Flagura Full Platform & Multi-Language SDK E2E Automated Verification${NC}"
echo -e "${BOLD}${BLUE}========================================================================${NC}\n"

# Check required runtimes
echo -e "${YELLOW}🔍 Checking test runtimes...${NC}"
command -v go >/dev/null 2>&1 || { echo -e "${RED}❌ Go is required but not found.${NC}"; exit 1; }
command -v node >/dev/null 2>&1 || { echo -e "${RED}❌ Node.js is required but not found.${NC}"; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo -e "${RED}❌ Python 3 is required but not found.${NC}"; exit 1; }
echo -e "${GREEN}✓ All core runtimes detected (Go $(go version | awk '{print $3}'), Node $(node -v), Python $(python3 --version | awk '{print $2}'))${NC}\n"

# Ensure TypeScript SDK dependencies are available
echo -e "${YELLOW}📦 Verifying TypeScript SDK build...${NC}"
if [ ! -d "sdks/js/dist" ]; then
    echo "Building JS/TS SDK..."
    (cd sdks/js && npm run build)
fi
echo -e "${GREEN}✓ TypeScript SDK ready.${NC}\n"

# Execute Full Go E2E Suite (which spins up ephemeral server and runs Go, TS, Python, and Rust verifications)
echo -e "${YELLOW}🧪 Executing E2E Test Suite (tests/e2e)...${NC}"
go test -v -count=1 ./tests/e2e/...

echo -e "\n${BOLD}${GREEN}========================================================================${NC}"
echo -e "${BOLD}${GREEN}🎉 ALL END-TO-END TESTS & MULTI-LANGUAGE SDK VERIFICATIONS PASSED!${NC}"
echo -e "${BOLD}${GREEN}========================================================================${NC}\n"
