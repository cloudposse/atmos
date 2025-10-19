#!/bin/bash

# Script to check and fix git index corruption in all worktrees
set -e

CONDUCTOR_DIR=".conductor"

if [ ! -d "$CONDUCTOR_DIR" ]; then
    echo "Error: $CONDUCTOR_DIR directory not found"
    exit 1
fi

echo "Checking and fixing git index corruption in all worktrees..."

# Get list of worktree directories from git worktree list
worktree_dirs=$(git worktree list --porcelain | grep "worktree " | sed 's/worktree //' | grep "$CONDUCTOR_DIR")

total_count=$(echo "$worktree_dirs" | wc -l)
current=0

for worktree_path in $worktree_dirs; do
    current=$((current + 1))
    worktree_name=$(basename "$worktree_path")
    
    echo "[$current/$total_count] Checking worktree: $worktree_name"
    
    if [ ! -d "$worktree_path" ]; then
        echo "  Warning: Directory $worktree_path does not exist, skipping..."
        continue
    fi
    
    cd "$worktree_path"
    
    # Check git status
    git_status_output=$(git status --porcelain 2>/dev/null || echo "ERROR")
    
    # Check for corruption patterns
    needs_fix=false
    
    if [ "$git_status_output" = "ERROR" ]; then
        echo "  Git status failed - index corruption detected"
        needs_fix=true
    elif echo "$git_status_output" | grep -q "^D "; then
        # Check if there are many staged deletions
        deletion_count=$(echo "$git_status_output" | grep "^D " | wc -l)
        if [ "$deletion_count" -gt 10 ]; then
            echo "  Many staged deletions detected ($deletion_count files) - likely index corruption"
            needs_fix=true
        fi
    fi
    
    if [ "$needs_fix" = true ]; then
        echo "  Attempting to fix..."
        
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
        new_status=$(git status --porcelain 2>/dev/null || echo "ERROR")
        if [ "$new_status" != "ERROR" ] && ! echo "$new_status" | grep -q "^D "; then
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

echo "Completed checking all worktrees"