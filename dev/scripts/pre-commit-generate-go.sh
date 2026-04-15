#!/bin/bash
# Check if any changed files contain //go:generate comments
# If so, run make generate-go (without file arguments)

for file in "$@"; do
    if grep -q "//go:generate" "$file" 2>/dev/null; then
        make generate-go
        exit 0
    fi
done
