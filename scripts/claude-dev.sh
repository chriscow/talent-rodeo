#!/bin/bash
# scripts/claude-dev.sh - Start Claude development session
set -e

PROMPT_FILE=${1:-}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${PURPLE}🤖 Starting Claude development session...${NC}"

# Check if the isolated environment is running
if ! docker-compose -f docker-compose.isolated.yml ps | grep -q "Up"; then
    echo -e "${RED}❌ Isolated environment is not running.${NC}"
    echo -e "${YELLOW}💡 Start it first with: ./scripts/start-isolated.sh${NC}"
    exit 1
fi

# Check if prompt file exists and is readable
if [ -n "$PROMPT_FILE" ]; then
    if [ ! -f "$PROMPT_FILE" ]; then
        echo -e "${RED}❌ Prompt file not found: $PROMPT_FILE${NC}"
        echo -e "${YELLOW}💡 Create a prompt file or run without arguments for interactive session${NC}"
        exit 1
    fi
    
    echo -e "${CYAN}📋 Using prompt file: $PROMPT_FILE${NC}"
    
    # Copy prompt file to a temporary location that the container can access
    mkdir -p ./prompts/temp
    cp "$PROMPT_FILE" ./prompts/temp/current-prompt.txt
    
    # Start Claude session with prompt
    docker-compose -f docker-compose.isolated.yml exec claude /usr/local/bin/claude-session.sh /workspace/prompts/temp/current-prompt.txt
    
    # Cleanup
    rm -f ./prompts/temp/current-prompt.txt
    rmdir ./prompts/temp 2>/dev/null || true
else
    echo -e "${CYAN}💬 Starting interactive Claude session...${NC}"
    
    # Start interactive Claude session
    docker-compose -f docker-compose.isolated.yml exec claude /usr/local/bin/claude-session.sh
fi 