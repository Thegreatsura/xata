#!/bin/bash
# Pass changed Go files to make fmt-go using FILES variable
# Excludes generated files with "// Code generated ... DO NOT EDIT." marker

if [ $# -eq 0 ]; then
    make fmt-go
else
    # Filter out generated files which contain // Code generated... comments
    # grep -L returns files that don't match the pattern (portable across macOS/Linux)
    NON_GENERATED_FILES=$(printf '%s\0' "$@" | xargs -0 grep -L "^// Code generated .* DO NOT EDIT\.$" 2>/dev/null | tr '\n' ' ')

    # Only run fmt-go if there are non-generated files to format
    if [ -n "${NON_GENERATED_FILES// /}" ]; then
        make fmt-go FILES="$NON_GENERATED_FILES"
    fi
fi
