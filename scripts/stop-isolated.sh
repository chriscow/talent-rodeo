#!/bin/bash
# scripts/stop-isolated.sh - Stop isolated development environment
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

CLEAN=${1:-false}

echo -e "${BLUE}🛑 Stopping isolated tr-chat-interface environment${NC}"

# Stop and remove containers
echo -e "${YELLOW}⏹️ Stopping services...${NC}"
docker-compose -f docker-compose.isolated.yml down

if [ "$CLEAN" = "--clean" ] || [ "$CLEAN" = "-c" ]; then
    echo -e "${YELLOW}🧹 Cleaning up volumes and images...${NC}"
    
    # Remove volumes
    docker-compose -f docker-compose.isolated.yml down -v
    
    # Remove built images
    docker image rm tr-chat-interface_dev 2>/dev/null || true
    docker image rm tr-chat-interface_claude 2>/dev/null || true
    
    # Clean up any dangling images
    docker image prune -f
    
    echo -e "${GREEN}✅ Environment stopped and cleaned up${NC}"
else
    echo -e "${GREEN}✅ Environment stopped${NC}"
    echo -e "${CYAN}💡 To clean up volumes and images, run: ./scripts/stop-isolated.sh --clean${NC}"
fi

echo ""
echo -e "${CYAN}📋 Useful commands:${NC}"
echo -e "  ${YELLOW}Restart:${NC}        ./scripts/start-isolated.sh"
echo -e "  ${YELLOW}View containers:${NC} docker ps -a"
echo -e "  ${YELLOW}View images:${NC}     docker images"
echo -e "  ${YELLOW}Clean all:${NC}      docker system prune -a" 