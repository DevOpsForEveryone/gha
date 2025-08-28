#!/bin/bash
set -e

echo "🔧 Setting up pre-commit hooks..."

# Install pre-commit if not available
if ! command -v pre-commit &> /dev/null; then
    echo "Installing pre-commit..."
    if command -v pip3 &> /dev/null; then
        pip3 install pre-commit
    elif command -v pip &> /dev/null; then
        pip install pre-commit
    else
        echo "❌ pip not found. Please install Python and pip first."
        exit 1
    fi
fi

# Install hooks
pre-commit install

echo "✅ Pre-commit hooks installed successfully!"
echo "💡 Run 'pre-commit run --all-files' to check all files"