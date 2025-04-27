# Base image with Node.js
FROM node:18-slim

# Install OpenJDK (required for Firebase emulators)
RUN apt-get update && apt-get install -y \
    default-jre-headless \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install Firebase CLI tools
RUN npm install -g firebase-tools

# Create app directory
WORKDIR /app

# Copy configuration files
COPY firebase.json .
COPY .firebaserc .

# Set environment variables
ENV HOST=0.0.0.0
ENV FIRESTORE_EMULATOR_HOST="0.0.0.0:8200"

# Expose ports
# 4000: Emulator UI
# 8200: Firestore
EXPOSE 4040 8200

# Create a simple entrypoint script
RUN echo '#!/bin/sh\n\
firebase emulators:start --only firestore\
' > /app/entrypoint.sh && chmod +x /app/entrypoint.sh

# Set the entrypoint
ENTRYPOINT ["/app/entrypoint.sh"]