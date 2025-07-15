#!/bin/bash

# Script to run the hybrid completion testing framework demo

echo "=== Hybrid Completion Testing Framework Demo ==="
echo ""

# Check if OpenAI API key is set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ OpenAI API key not found!"
    echo "Please set your OpenAI API key:"
    echo "export OPENAI_API_KEY=your_key_here"
    echo ""
    exit 1
fi

echo "✅ OpenAI API key found"

# Load environment variables
echo "📋 Loading environment variables..."
set -a
source .env 2>/dev/null || true
set +a

# Set Firestore emulator host for testing
export FIRESTORE_EMULATOR_HOST=localhost:8200

echo "🚀 Starting framework demo..."
echo ""

# Run the demo
go run -tags=integration demo_test.go

echo ""
echo "✅ Demo completed!"
echo ""
echo "You can also run the integration tests directly with:"
echo "OPENAI_API_KEY=your_key go test -tags=integration ./internal/chat"