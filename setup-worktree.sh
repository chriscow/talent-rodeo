#!/bin/bash
# setup-worktree.sh
WORKTREE_PATH=$1
BRANCH=$2

git worktree add "$WORKTREE_PATH" "$BRANCH"

# Copy common untracked files
cp .env "$WORKTREE_PATH/" 2>/dev/null || echo "No .env file to copy"
cp .prod.env "$WORKTREE_PATH/" 2>/dev/null || echo "No .prod.env file to copy"

echo "Worktree setup complete at $WORKTREE_PATH"