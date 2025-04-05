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

# Create directory for secrets and copy Firebase credentials
RUN mkdir -p /app/.secrets
COPY .secrets/talent-rodeo-prod-firebase-adminsdk-fbsvc-916e43968d.json /app/.secrets/
ENV FIREBASE_CREDENTIALS_FILE=/app/.secrets/talent-rodeo-prod-firebase-adminsdk-fbsvc-916e43968d.json

ENTRYPOINT ["/usr/local/bin/tr-web"]
