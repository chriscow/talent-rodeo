#!/bin/bash

# Check if an environment file was provided as an argument
ENV_FILE="${1:-.prod.env}"

# Check if the env file exists
if [ ! -f "$ENV_FILE" ]; then
    echo "Error: $ENV_FILE file not found"
    exit 1
fi

# Create a temporary file for secrets
SECRETS_FILE=$(mktemp)

# Read the env file line by line and build the secrets file
while IFS= read -r line; do
    # Skip empty lines and comments
    if [[ -n "$line" && ! "$line" =~ ^[[:space:]]*# ]]; then
        # Extract key and value
        if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
            key="${BASH_REMATCH[1]}"
            value="${BASH_REMATCH[2]}"
            
            # Skip FIREBASE_CREDENTIALS as we'll handle it separately
            if [ "$key" != "FIREBASE_CREDENTIALS" ]; then
                echo "$key=$value" >> "$SECRETS_FILE"
            fi
        fi
    fi
done < "$ENV_FILE"

# Add Firebase credentials
echo "Setting up Firebase credentials"
credentials=$(cat .secrets/talent-rodeo-prod-firebase-adminsdk-fbsvc-916e43968d.json | jq -c .)
echo "FIREBASE_CREDENTIALS=$credentials" >> "$SECRETS_FILE"

# Import all secrets at once
echo "Importing all secrets from $ENV_FILE..."
fly secrets import < "$SECRETS_FILE"

# Clean up
rm "$SECRETS_FILE"

echo "Successfully set Fly.io secrets from $ENV_FILE" 