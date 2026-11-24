#!/bin/sh

set -eu

problems=$(find guest test scripts -name '*.ps1' -exec awk '
    /^[[:space:]]*<#[[:space:]]*$/ { in_help = 1; has_synopsis = 0; start = FNR; next }
    in_help && /^[[:space:]]*#>[[:space:]]*$/ {
        if (!has_synopsis) print FILENAME ":" start ": help block without .SYNOPSIS"
        in_help = 0
        next
    }
    in_help { if ($0 ~ /^[[:space:]]*\.SYNOPSIS[[:space:]]*$/) has_synopsis = 1; next }
    /^[[:space:]]*#Requires/ { next }
    {
        line = $0
        gsub(/\047[^\047]*\047/, "", line)
        gsub(/"[^"]*"/, "", line)
        if (line ~ /(^|[[:space:]])(#|<#)/) print FILENAME ":" FNR ":" $0
    }
' {} +)

if [ -n "$problems" ]; then
    echo "PowerShell comments are only allowed as comment-based help (INVARIANTS.md I19):"
    echo "$problems"
    exit 1
fi
