#!/bin/bash
# setup-worktree.sh
WORKTREE_PATH=$1
BRANCH=$2

if [ -z "$WORKTREE_PATH" ] || [ -z "$BRANCH" ]; then
  echo "Usage: $0 <worktree-path> <branch>" >&2
  echo "Both worktree path and branch name are required." >&2
  exit 1
fi

git worktree add "$WORKTREE_PATH" "$BRANCH"

# Copy common untracked files
cp .env "$WORKTREE_PATH/" 2>/dev/null || echo "No .env file to copy"
cp .prod.env "$WORKTREE_PATH/" 2>/dev/null || echo "No .prod.env file to copy"
cp -R ./testdata "$WORKTREE_PATH/" 2>/dev/null || echo "No testdata directory to copy"
cp -R .secrets "$WORKTREE_PATH/" 2>/dev/null || echo "No secrets directory to copy"
cp -R .vscode "$WORKTREE_PATH/" 2>/dev/null || echo "No .vscode directory to copy"
cp -R .claude "$WORKTREE_PATH/" 2>/dev/null || echo "No .claude directory to copy"


echo "Worktree setup complete at $WORKTREE_PATH"