#!/bin/bash

# Simple script to run just the integration tests

echo "=== Running Integration Tests ==="
echo ""


# Load environment variables
echo "📋 Loading environment variables..."
set -a
source .env 2>/dev/null || true
set +a

# Set Firestore emulator host for testing
export FIRESTORE_EMULATOR_HOST=localhost:8200


# Check if OpenAI API key is set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ OpenAI API key not found!"
    echo "Please set your OpenAI API key:"
    echo "export OPENAI_API_KEY=your_key_here"
    echo ""
    exit 1
fi

echo "✅ OpenAI API key found"

echo "🚀 Running integration tests..."
echo ""

# Run the integration tests
go test -tags=integration -v ./internal/chat -run TestHybridCompletion

echo ""
echo "✅ Tests completed!"