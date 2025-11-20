#!/bin/bash
# This script cleans all the built binaries from the plugins

echo "Cleaning all plugin builds"
for dir in */; do
    if [ -d "$dir" ]; then
        echo "Cleaning directory: $dir"
        cd "$dir"
        
        # Detect platform and set executable name
        platform=$(uname)
        if [ "$platform" = "Linux" ]; then
            exe_name="${dir%/}"
        else
            exe_name="${dir%/}.exe"
        fi
        
        # Remove the executable
        if [ -f "$exe_name" ]; then
            echo "Removing $exe_name"
            rm "$exe_name"
        fi
        
        # Return to the parent directory
        cd ..
    fi
done

echo "Clean process completed for all directories."
