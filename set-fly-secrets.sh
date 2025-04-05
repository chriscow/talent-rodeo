#!/bin/bash

# Check if an environment file was provided as an argument
ENV_FILE="${1:-.env}"

# Check if the env file exists
if [ ! -f "$ENV_FILE" ]; then
    echo "Error: $ENV_FILE file not found"
    exit 1
fi

# Initialize an array to store secrets
declare -a secret_args=()

# Read the env file line by line
while IFS= read -r line; do
    # Skip empty lines and comments
    if [[ -n "$line" && ! "$line" =~ ^[[:space:]]*# ]]; then
        # Add the line to secrets array
        secret_args+=("$line")
    fi
done < "$ENV_FILE"

# Check if we found any secrets
if [ ${#secret_args[@]} -eq 0 ]; then
    echo "No valid environment variables found in $ENV_FILE"
    exit 1
fi

# Convert array to space-separated string and pass to fly secrets set
fly secrets set "${secret_args[@]}"

echo "Successfully set Fly.io secrets from $ENV_FILE" 