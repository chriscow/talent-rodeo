ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm as builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -o /tr-web ./cmd/web


FROM debian:bookworm
RUN apt-get update -y && apt-get install -y ca-certificates
COPY --from=builder /tr-web /usr/local/bin/

# Create directory for secrets
RUN mkdir -p /app/.secrets
ENV FIREBASE_CREDENTIALS_FILE=/app/.secrets/firebase-credentials.json

# Add a script to write credentials at startup
COPY <<EOF /usr/local/bin/docker-entrypoint.sh
#!/bin/sh
echo "$FIREBASE_CREDENTIALS" > $FIREBASE_CREDENTIALS_FILE
exec /usr/local/bin/tr-web
EOF

RUN chmod +x /usr/local/bin/docker-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
