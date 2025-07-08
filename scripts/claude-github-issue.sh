#!/bin/bash
# scripts/claude-github-issue.sh - Start Claude development session with GitHub issue
set -e

ISSUE_NUMBER=${1:-}
REPO=${2:-chriscow/talent-rodeo}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

if [ -z "$ISSUE_NUMBER" ]; then
    echo -e "${RED}❌ Usage: ./scripts/claude-github-issue.sh <issue-number> [repo]${NC}"
    echo -e "${CYAN}📋 Example: ./scripts/claude-github-issue.sh 42${NC}"
    echo -e "${CYAN}📋 Example: ./scripts/claude-github-issue.sh 42 chriscow/talent-rodeo${NC}"
    exit 1
fi

echo -e "${PURPLE}🤖 Starting Claude development session for GitHub issue #$ISSUE_NUMBER...${NC}"

# Check if the isolated environment is running
if ! docker-compose -f docker-compose.isolated.yml ps | grep -q "Up"; then
    echo -e "${RED}❌ Isolated environment is not running.${NC}"
    echo -e "${YELLOW}💡 Start it first with: ./scripts/start-isolated.sh${NC}"
    exit 1
fi

# Check if GitHub token is available in environment
if [ -z "$GITHUB_TOKEN" ]; then
    echo -e "${YELLOW}⚠️  GITHUB_TOKEN not set in environment${NC}"
    echo -e "${CYAN}💡 For private repos, set GITHUB_TOKEN before starting:${NC}"
    echo -e "${CYAN}   export GITHUB_TOKEN=ghp_your_token${NC}"
    echo -e "${CYAN}   ./scripts/start-isolated.sh${NC}"
    echo ""
    echo -e "${CYAN}🔄 Continuing anyway (will work for public repos)...${NC}"
fi

echo -e "${CYAN}🔍 Starting GitHub issue workflow...${NC}"

# Start Claude session with GitHub issue workflow
docker-compose -f docker-compose.isolated.yml exec claude /usr/local/bin/github-issue.sh "$ISSUE_NUMBER" "$REPO" 