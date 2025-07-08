# Isolated Development Environment

This document explains how to work with the fully isolated development environment for talent-rodeo, designed to work completely independently of your host system and to enable AI-assisted development with Claude.

## 🎯 Overview

The isolated development environment provides:
- ✅ **Complete isolation** from host filesystem
- ✅ **Cloud deployment ready** (Digital Ocean, Fly.io, etc.)
- ✅ **Claude-friendly** with dedicated AI development container
- ✅ **All services included** (PostgreSQL, Firestore emulator)
- ✅ **Branch switching** support
- ✅ **Secret management** with multiple approaches

## 🚀 Quick Start

### Prerequisites
- Docker and Docker Compose installed
- Git repository access
- Environment variables or `.env` file with secrets

### 1. First Time Setup

```bash
# Clone the repository (if not already done)
git clone https://github.com/chriscow/talent-rodeo.git
cd talent-rodeo

# Create your environment file
cp .prod.env .env
# Edit .env with your actual API keys and secrets

# Start the isolated environment
./scripts/start-isolated.sh
```

### 2. Start Claude Development Session

```bash
# Interactive session
./scripts/claude-dev.sh

# With initial prompt
./scripts/claude-dev.sh prompts/examples/feature-request.txt
```

### 3. Stop Environment

```bash
# Stop services (keeps data)
./scripts/stop-isolated.sh

# Stop and clean up everything
./scripts/stop-isolated.sh --clean
```

## 📋 Available Scripts

| Script | Purpose | Usage |
|--------|---------|-------|
| `./scripts/start-isolated.sh` | Start the isolated environment | `./scripts/start-isolated.sh [branch] [env-file] [mode]` |
| `./scripts/claude-dev.sh` | Start Claude development session | `./scripts/claude-dev.sh [prompt-file]` |
| `./scripts/claude-github-issue.sh` | Start Claude with GitHub issue | `./scripts/claude-github-issue.sh <issue-number> [repo]` |
| `./scripts/claude-submit-pr.sh` | Submit PR from container | `./scripts/claude-submit-pr.sh <issue-number> [options]` |
| `./scripts/stop-isolated.sh` | Stop the environment | `./scripts/stop-isolated.sh [--clean]` |

## 🔧 Detailed Usage

### Starting the Environment

```bash
# Start with default settings (main branch, .env file)
./scripts/start-isolated.sh

# Start with specific branch
./scripts/start-isolated.sh feature/new-feature

# Start with custom env file
./scripts/start-isolated.sh main .dev.env

# Start in cloud mode (uses environment variables)
./scripts/start-isolated.sh main .env cloud
```

### Working with Different Branches

```bash
# Start environment with a specific branch
./scripts/start-isolated.sh feature/ai-improvements

# The container will automatically clone and checkout the specified branch
# All work happens inside the isolated container
```

### Claude Development Sessions

#### Interactive Session
```bash
./scripts/claude-dev.sh
```
This starts an interactive bash session where Claude can:
- Explore the codebase
- Run tests and builds
- Make changes
- Commit code

#### Prompt-Driven Session
```bash
./scripts/claude-dev.sh prompts/examples/feature-request.txt
```
This starts Claude with a specific prompt/task loaded.

#### GitHub Issue Workflow (Recommended)
```bash
# Set your GitHub token first (for private repos)
export GITHUB_TOKEN=ghp_your_token_here

# Start Claude with a specific GitHub issue
./scripts/claude-github-issue.sh 42

# Claude will:
# 1. Fetch the issue details automatically
# 2. Create a feature branch (e.g., issue-42-fix-login-bug)
# 3. Provide full context about the issue
# 4. Help you implement the solution
# 5. Submit a PR when ready using: submit-pr 42
```

This workflow is perfect for:
- Working on existing GitHub issues
- Automatic branch creation and naming
- Seamless PR submission when work is complete
- Maintaining proper issue linking and closure

## 📝 Creating Prompts for Claude

Create prompt files in the `prompts/` directory to give Claude specific tasks:

### Example: Feature Request
```bash
# Create a new prompt file
cat > prompts/add-user-dashboard.txt << 'EOF'
I need you to add a user dashboard feature to the application.

Requirements:
- Create a new dashboard page accessible from the main navigation
- Show user statistics (jobs applied, interviews scheduled, etc.)
- Use the existing design system
- Add appropriate tests
- Follow established code patterns

Please analyze the codebase first, then implement this feature step by step.
EOF

# Use it with Claude
./scripts/claude-dev.sh prompts/add-user-dashboard.txt
```

## 🔐 Secret Management

### GitHub Authentication (For Issue Workflow)

For the GitHub issue workflow to work with private repositories, you need to authenticate:

#### Option 1: Personal Access Token (Recommended)
```bash
# Create a Personal Access Token at: https://github.com/settings/tokens
# Scopes needed: repo, read:org, workflow

export GITHUB_TOKEN=ghp_your_token_here
./scripts/start-isolated.sh
```

#### Option 2: GitHub CLI Login (Alternative)
```bash
# Login to GitHub CLI first
gh auth login

# Then start the environment (token will be detected automatically)
./scripts/start-isolated.sh
```

#### Option 3: Add to .env File
```bash
# Add to your .env file
echo "GITHUB_TOKEN=ghp_your_token_here" >> .env
./scripts/start-isolated.sh
```

### Method 1: Local .env File (Development)

Create a `.env` file with your secrets:

```bash
# Required API Keys
OPENAI_API_KEY=sk-your-key-here
ANTHROPIC_API_KEY=your-anthropic-key

# Firebase Credentials (JSON string)
FIREBASE_CREDENTIALS={"type":"service_account",...}

# Optional: Base64 encoded Firebase credentials
FIREBASE_CREDENTIALS_B64=REVOKED_CREDENTIAL

# Other configuration
LINKEDIN_CLIENT_ID=your-client-id
LINKEDIN_CLIENT_SECRET=your-client-secret
```

### Method 2: Environment Variables (Cloud)

For cloud deployments, export environment variables:

```bash
export OPENAI_API_KEY="sk-your-key"
export ANTHROPIC_API_KEY="your-key"
export FIREBASE_CREDENTIALS='{"type":"service_account",...}'

# Start in cloud mode
./scripts/start-isolated.sh main .env cloud
```

### Method 3: Cloud Provider Secrets

#### Fly.io
```bash
fly secrets set OPENAI_API_KEY="sk-your-key" -a tr-chat-dev
fly secrets set ANTHROPIC_API_KEY="your-key" -a tr-chat-dev
```

#### Digital Ocean
```bash
# Set environment variables in your droplet
docker run -d \
  --name tr-chat-dev \
  -p 2304:2304 \
  -e OPENAI_API_KEY="$OPENAI_API_KEY" \
  -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  tr-chat-isolated:latest
```

## 🌐 Available Services

When the environment is running, you can access:

| Service | URL | Description |
|---------|-----|-------------|
| Web Application | http://localhost:2304 | Main talent-rodeo app |
| Firestore Emulator UI | http://localhost:8200 | Firebase emulator interface |
| PostgreSQL | localhost:5432 | Database (postgres/postgres) |

## 🛠️ Development Workflow

### 1. Standard Development
```bash
# Start environment
./scripts/start-isolated.sh

# Start Claude session
./scripts/claude-dev.sh

# Inside Claude session:
cd /workspace
git checkout -b feature/my-feature
# Make changes
task test
task live:server  # Test manually
git add .
git commit -m "Add new feature"
git push origin feature/my-feature
```

### 2. AI-Assisted Development
```bash
# Create a specific prompt
cat > prompts/my-task.txt << 'EOF'
Please implement user authentication using the existing patterns.
Look at how LinkedIn OAuth is currently implemented and add 
support for Google OAuth as well.
EOF

# Give Claude the task
./scripts/claude-dev.sh prompts/my-task.txt

# Claude will analyze, plan, and implement
```

### 3. Code Review Workflow
```bash
# Have Claude review the codebase
./scripts/claude-dev.sh prompts/examples/code-review.txt

# Claude will provide detailed analysis and recommendations
```

### 4. GitHub Issue-Driven Development (Recommended)
```bash
# 1. Set up GitHub authentication
export GITHUB_TOKEN=ghp_your_token_here

# 2. Start environment with your desired branch
./scripts/start-isolated.sh main

# 3. Start Claude with a specific GitHub issue
./scripts/claude-github-issue.sh 123

# Inside the Claude session, Claude will:
# - Analyze the issue automatically
# - Create a feature branch: issue-123-fix-something
# - Understand requirements and context
# - Implement the solution step by step
# - Run tests to verify the solution

# 4. When Claude is ready, submit the PR
submit-pr 123

# This automatically:
# - Commits all changes with a descriptive message
# - Pushes the branch to GitHub
# - Creates a PR with proper title and description
# - Links the PR to the original issue
```

This workflow is particularly powerful because:
- ✅ **Automatic context loading** from GitHub issues
- ✅ **Proper branch naming** and management
- ✅ **Seamless PR creation** with issue linking
- ✅ **Complete isolation** from your local system
- ✅ **Cloud-ready** for remote development

## 🐳 Container Architecture

### Services
- **dev**: Main development container with live reload
- **claude**: Dedicated container for AI development sessions
- **db**: PostgreSQL database
- **Firestore emulator**: Built into dev container

### Volumes
- **dev-workspace**: Isolated workspace (contains git repo)
- **dev-go-cache**: Go module cache for faster builds
- **postgres-data**: Database persistence

### Networking
All services communicate via a dedicated Docker network (`tr-isolated`).

## 🌍 Cloud Deployment

### Fly.io Deployment
```bash
# Build for Fly.io
docker build -f Dockerfile.isolated \
  --build-arg BRANCH=main \
  -t tr-chat-dev .

# Deploy
fly deploy --dockerfile Dockerfile.isolated \
  --build-arg BRANCH=main \
  --build-arg REPO_URL=https://github.com/chriscow/talent-rodeo.git
```

### Digital Ocean Droplet
```bash
# On your droplet
git clone https://github.com/chriscow/talent-rodeo.git
cd talent-rodeo

# Set environment variables
export OPENAI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"

# Start environment
./scripts/start-isolated.sh main .env cloud
```

## 🔍 Troubleshooting

### Environment Won't Start
```bash
# Check Docker is running
docker info

# Check for port conflicts
docker ps
lsof -i :2304

# View logs
docker-compose -f docker-compose.isolated.yml logs -f
```

### Secrets Not Working
```bash
# Verify .env file exists and has correct format
cat .env

# Check environment variables in container
docker-compose -f docker-compose.isolated.yml exec dev env | grep -E "(OPENAI|ANTHROPIC|FIREBASE)"
```

### Claude Session Issues
```bash
# Ensure environment is running
docker-compose -f docker-compose.isolated.yml ps

# Check container logs
docker-compose -f docker-compose.isolated.yml logs claude
```

### Cleanup Everything
```bash
# Stop and remove everything
./scripts/stop-isolated.sh --clean

# Nuclear option - remove all Docker data
docker system prune -a --volumes
```

## 📚 Advanced Usage

### Custom Git Repositories
```bash
# Use a fork or different repository
REPO_URL=https://github.com/yourfork/talent-rodeo.git ./scripts/start-isolated.sh
```

### Private Repositories
```bash
# Use GitHub token for private repos
GITHUB_TOKEN=ghp_your_token ./scripts/start-isolated.sh
```

### Development with Multiple Branches
```bash
# Start environment with feature branch
./scripts/start-isolated.sh feature/experiment-1

# In another terminal, start another environment
./scripts/start-isolated.sh feature/experiment-2

# Each runs in complete isolation
```

### Connecting External Tools
```bash
# Access database from host
psql -h localhost -p 5432 -U postgres -d talent_rodeo

# Access Firestore emulator
curl http://localhost:8200
```

## 🤖 Working with Claude

### Best Practices for Prompts

1. **Be Specific**: Clearly describe what you want
2. **Provide Context**: Explain the business requirements
3. **Set Constraints**: Mention performance, security, or style requirements
4. **Request Analysis**: Ask Claude to understand the codebase first

### Example Interaction Flow

```bash
# 1. Start Claude with a prompt
./scripts/claude-dev.sh prompts/examples/feature-request.txt

# 2. Claude analyzes the codebase
# 3. Claude asks clarifying questions
# 4. Claude implements the feature
# 5. Claude runs tests
# 6. Claude commits the changes
```

### Iterative Development

```bash
# Start a session
./scripts/claude-dev.sh

# Inside the session:
# > Analyze the user authentication flow
# > What security improvements can we make?
# > Implement rate limiting on login attempts
# > Add comprehensive tests for the new functionality
```

## 💡 Tips and Best Practices

1. **Always start with analysis**: Have Claude understand the codebase before making changes
2. **Use focused prompts**: Break large tasks into smaller, specific requests
3. **Review changes**: Always review Claude's code before committing
4. **Test thoroughly**: Use `task test` and `task live:server` to verify changes
5. **Backup important work**: Commit frequently during development sessions
6. **Use branches**: Create feature branches for experimentation

## 📖 Additional Resources

- [Go Coding Guidelines](go-coding-guidelines.md)
- [Frontend Guidelines](front-end-guidelines.md)
- [Firestore Best Practices](firestore.md)
- [Main README](README.md)

## 🆘 Support

If you encounter issues:

1. Check the troubleshooting section above
2. Review container logs: `docker-compose -f docker-compose.isolated.yml logs`
3. Ensure all prerequisites are installed
4. Try a clean restart: `./scripts/stop-isolated.sh --clean && ./scripts/start-isolated.sh` 