#!/bin/bash

# Iterate over all directories in the current directory
for dir in */; do
    if [ -d "$dir" ]; then
        echo "Processing directory: $dir"
        cd "$dir"
        
        # Execute go mod tidy
        echo "Running go mod tidy in $dir"
        go mod tidy
        
        # Execute go build
        echo "Running go build in $dir"
        go build
        
        # Return to the parent directory
        cd ..
    fi
done

echo "Build process completed for all directories."