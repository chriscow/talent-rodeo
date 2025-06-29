#!/bin/bash

# Usage script for ccusage
# Default date if none provided
DEFAULT_DATE="20250601"

# Use the first argument as the date, or default if not provided
SINCE_DATE="${1:-$DEFAULT_DATE}"

# Run the npx command with the specified date
npx ccusage@latest --since "$SINCE_DATE" 