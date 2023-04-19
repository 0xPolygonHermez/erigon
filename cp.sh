#!/bin/bash

# Path to the file containing the commit hashes
file_path="/Users/maxrevitt/github/rebase.txt"

commits=()

# Read commit hashes into an array
while IFS= read -r line; do
    commits+=("$line")
done < "$file_path"

# Reverse the order of the array
reversed_commits=()
for (( idx=${#commits[@]}-1 ; idx>=0 ; idx-- )) ; do
    reversed_commits+=( "${commits[idx]}" )
done

# Cherry-pick each commit in the reversed order
for commit in "${reversed_commits[@]}"; do
    # Check if the commit is a merge commit
    parent_count=$(git cat-file -p "$commit" | grep -c '^parent')

    if [ "$parent_count" -gt 1 ]; then
        echo "Cherry-picking merge commit (assuming first parent): $commit"
        git cherry-pick -m 1 "$commit"
    else
        echo "Cherry-picking commit: $commit"
        git cherry-pick "$commit"
    fi

    cherry_pick_status=$?

    # If cherry-pick fails (conflict), check if it's an empty commit
    if [ $cherry_pick_status -ne 0 ]; then
        if git diff --quiet && git diff --cached --quiet; then
            echo "Cherry-pick resulted in no changes, skipping empty commit."
            git cherry-pick --skip
        else
            echo "Conflict detected in $commit. Favoring incoming changes..."
            git checkout --theirs .
            git add .
            git cherry-pick --continue

            if [ $? -ne 0 ]; then
                echo "Failed to continue cherry-picking after resolving conflicts. Please check manually."
                exit 1
            fi
        fi
    fi
done

echo "Cherry-picking completed."
