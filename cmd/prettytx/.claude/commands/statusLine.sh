#!/bin/bash

# Get current directory (abbreviated home as ~)
current_dir="${PWD/#$HOME/\~}"

# Get git branch and status if in a git repo
if git rev-parse --git-dir > /dev/null 2>&1; then
    branch=$(git branch --show-current 2>/dev/null)

    # Check for uncommitted changes
    if [[ -n $(git status --porcelain 2>/dev/null) ]]; then
        status="*"
    else
        status=""
    fi

    echo "${current_dir} (${branch}${status})"
else
    echo "${current_dir}"
fi
