#!/bin/bash

# Script to fix git index corruption in worktrees
set -e

CONDUCTOR_DIR=".conductor"

if [ ! -d "$CONDUCTOR_DIR" ]; then
    echo "Error: $CONDUCTOR_DIR directory not found"
    exit 1
fi

echo "Fixing git index corruption in worktrees..."

# Get list of worktree directories from git worktree list
worktree_dirs=$(git worktree list --porcelain | grep "worktree " | sed 's/worktree //' | grep "$CONDUCTOR_DIR")

total_count=$(echo "$worktree_dirs" | wc -l)
current=0

for worktree_path in $worktree_dirs; do
    current=$((current + 1))
    worktree_name=$(basename "$worktree_path")
    
    echo "[$current/$total_count] Processing worktree: $worktree_name"
    
    if [ ! -d "$worktree_path" ]; then
        echo "  Warning: Directory $worktree_path does not exist, skipping..."
        continue
    fi
    
    cd "$worktree_path"
    
    # Check if git index is corrupted
    if ! git status --porcelain > /dev/null 2>&1; then
        echo "  Git index appears corrupted, attempting to fix..."
        
        # Remove the index file if it exists
        if [ -f ".git/index" ]; then
            echo "    Removing corrupt index file..."
            rm ".git/index"
        fi
        
        # Reset the index
        echo "    Resetting index..."
        git reset --mixed HEAD > /dev/null 2>&1 || {
            echo "    Warning: git reset failed, trying alternative approach..."
            # Alternative: read-tree to rebuild index
            git read-tree HEAD > /dev/null 2>&1 || {
                echo "    Warning: git read-tree also failed"
            }
        }
        
        # Verify the fix worked
        if git status --porcelain > /dev/null 2>&1; then
            echo "    ✓ Index successfully fixed"
        else
            echo "    ✗ Index still appears corrupted"
        fi
    else
        echo "  ✓ Index appears healthy"
    fi
    
    # Return to main directory
    cd - > /dev/null
done

echo "Completed processing all worktrees"