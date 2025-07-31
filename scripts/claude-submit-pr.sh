#!/bin/bash
# scripts/claude-submit-pr.sh - Submit a pull request for completed work
set -e

ISSUE_NUMBER=${1:-}
COMMIT_MESSAGE=${2:-}
PR_TITLE=${3:-}
PR_BODY=${4:-}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

if [ -z "$ISSUE_NUMBER" ]; then
    echo -e "${RED}❌ Usage: ./scripts/claude-submit-pr.sh <issue-number> [commit-message] [pr-title] [pr-body]${NC}"
    echo -e "${CYAN}📋 Example: ./scripts/claude-submit-pr.sh 42${NC}"
    echo -e "${CYAN}📋 Example: ./scripts/claude-submit-pr.sh 42 \"Fix login bug\" \"Fix: Resolve login redirect issue\" \"Fixes #42\"${NC}"
    exit 1
fi

echo -e "${PURPLE}🚀 Submitting pull request for issue #$ISSUE_NUMBER...${NC}"

# Check if the isolated environment is running
if ! docker-compose -f docker-compose.isolated.yml ps | grep -q "Up"; then
    echo -e "${RED}❌ Isolated environment is not running.${NC}"
    echo -e "${YELLOW}💡 Start it first with: ./scripts/start-isolated.sh${NC}"
    exit 1
fi

# Execute PR submission inside the Claude container
docker-compose -f docker-compose.isolated.yml exec claude /usr/local/bin/submit-pr.sh "$ISSUE_NUMBER" "$COMMIT_MESSAGE" "$PR_TITLE" "$PR_BODY" 