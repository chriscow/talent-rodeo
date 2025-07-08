#!/bin/bash
# scripts/start-isolated.sh - Enhanced version for isolated development environment
set -e

# Default values
BRANCH=${1:-main}
ENV_FILE=${2:-.env}
MODE=${3:-development}  # development, production, cloud

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Starting isolated tr-chat-interface environment${NC}"
echo -e "${CYAN}📋 Branch: $BRANCH${NC}"
echo -e "${CYAN}📁 Env file: $ENV_FILE${NC}"
echo -e "${CYAN}🏗️ Mode: $MODE${NC}"
echo ""

# Function to check if Docker is running
check_docker() {
    if ! docker info > /dev/null 2>&1; then
        echo -e "${RED}❌ Docker is not running. Please start Docker and try again.${NC}"
        exit 1
    fi
}

# Function to check if docker-compose is available
check_docker_compose() {
    if ! command -v docker-compose &> /dev/null; then
        echo -e "${RED}❌ docker-compose is not installed. Please install it and try again.${NC}"
        exit 1
    fi
}

# Function to create template .env file
create_template_env() {
    echo -e "${YELLOW}💡 Creating template .env file...${NC}"
    
    cat > .env << 'ENVEOF'
# Isolated Development Environment Configuration
# Copy this file and fill in your actual values

# Required API Keys
OPENAI_API_KEY=your-openai-api-key-here
ANTHROPIC_API_KEY=your-anthropic-api-key-here

# LinkedIn OAuth (optional)
LINKEDIN_CLIENT_ID=your-linkedin-client-id
LINKEDIN_CLIENT_SECRET=your-linkedin-client-secret

# Email Configuration (optional)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password
FROM_EMAIL=noreply@talent-rodeo.com

# External APIs (optional)
VAPI_API_KEY=your-vapi-key
PINECONE_API_KEY=your-pinecone-key
PINECONE_ENVIRONMENT=your-pinecone-env
SCRAPINGDOG_API_KEY=your-scrapingdog-key

# Firebase Credentials (JSON string or base64 encoded)
FIREBASE_CREDENTIALS={"type":"service_account","project_id":"talent-rodeo",...}
# OR use base64 encoded version:
# FIREBASE_CREDENTIALS_B64=REVOKED_CREDENTIAL

# Security Keys (will use defaults if not specified)
SESSION_SECRET=your-session-secret-key
CSRF_SECRET=your-csrf-secret-key
JOB_ID_ENCRYPTION_KEY=your-encryption-key

# Database and Firebase
FIREBASE_PROJECT_ID=talent-rodeo
ENVEOF
    
    echo -e "${GREEN}✅ Template .env file created${NC}"
    echo -e "${YELLOW}✏️ Please edit .env with your actual values and run again${NC}"
    echo ""
    echo -e "${CYAN}💡 Tip: You can also copy from .prod.env if it exists:${NC}"
    echo -e "${CYAN}   cp .prod.env .env${NC}"
}

# Pre-flight checks
check_docker
check_docker_compose

case $MODE in
  "development")
    if [ -f "$ENV_FILE" ]; then
      echo -e "${GREEN}📝 Using local environment file: $ENV_FILE${NC}"
      export ENV_FILE_PATH=$ENV_FILE
      export BRANCH=$BRANCH
      
      echo -e "${BLUE}🔨 Building and starting services...${NC}"
      docker-compose -f docker-compose.isolated.yml up --build -d
      
      # Wait for services to be ready
      echo -e "${YELLOW}⏳ Waiting for services to be ready...${NC}"
      sleep 10
      
      echo -e "${GREEN}✅ Services started successfully!${NC}"
      
    elif [ -f ".prod.env" ]; then
      echo -e "${YELLOW}📝 .env not found, but .prod.env exists. Creating .env from template...${NC}"
      create_template_env
      echo -e "${CYAN}💡 You can also copy from .prod.env: ${YELLOW}cp .prod.env .env${NC}"
      exit 1
    else
      echo -e "${RED}❌ Environment file $ENV_FILE not found${NC}"
      create_template_env
      exit 1
    fi
    ;;
  "cloud")
    echo -e "${BLUE}☁️ Using environment variables from cloud provider${NC}"
    export BRANCH=$BRANCH
    docker-compose -f docker-compose.isolated.yml up --build -d
    ;;
  "production")
    echo -e "${PURPLE}🔒 Production mode not yet implemented${NC}"
    echo -e "${YELLOW}💡 Use development mode for now${NC}"
    exit 1
    ;;
  *)
    echo -e "${RED}❌ Unknown mode: $MODE${NC}"
    echo -e "${CYAN}Valid modes: development, cloud, production${NC}"
    exit 1
    ;;
esac

echo ""
echo -e "${GREEN}🎉 Environment started successfully!${NC}"
echo ""
echo -e "${CYAN}🌐 Available services:${NC}"
echo -e "  ${BLUE}Web Application:${NC} http://localhost:2304"
echo -e "  ${BLUE}Firestore UI:${NC}    http://localhost:8200"
echo -e "  ${BLUE}PostgreSQL:${NC}      localhost:5432"
echo ""
echo -e "${CYAN}🛠️ Management commands:${NC}"
echo -e "  ${YELLOW}View logs:${NC}           docker-compose -f docker-compose.isolated.yml logs -f"
echo -e "  ${YELLOW}Claude session:${NC}      ./scripts/claude-dev.sh"
echo -e "  ${YELLOW}Enter dev container:${NC} docker-compose -f docker-compose.isolated.yml exec dev bash"
echo -e "  ${YELLOW}Stop services:${NC}       docker-compose -f docker-compose.isolated.yml down"
echo ""
echo -e "${PURPLE}🤖 For Claude development:${NC}"
echo -e "  ${YELLOW}Start Claude session:${NC} ./scripts/claude-dev.sh [prompt-file]"
echo "" 