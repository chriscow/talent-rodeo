#!/bin/bash

# Load environment variables from .env file
export $(grep -v '^#' .env | xargs)

# Check if an email address is provided
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <email>"
    exit 1
fi

EMAIL=$1

# Retrieve the user ID based on the email
USER_ID=$(PGPASSWORD="$DB_PASSWORD" psql -U "$DB_USER" -h "$DB_HOST" -d "$DB_NAME" -t -c "SELECT id FROM users WHERE email = '$EMAIL';" | xargs)

# Check if the user ID was found
if [ -z "$USER_ID" ]; then
    echo "No user found with email '$EMAIL'."
    exit 1
fi

# Run the psql command to set the is_superuser column to true
PGPASSWORD="$DB_PASSWORD" psql -U "$DB_USER" -h "$DB_HOST" -d "$DB_NAME" -c "UPDATE users SET is_superuser = true WHERE id = '$USER_ID';"

# Check if the command was successful
if [ $? -eq 0 ]; then
    echo "User with email '$EMAIL' has been set as a superuser in the app."
else
    echo "Failed to set user as superuser."
fi