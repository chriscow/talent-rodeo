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

CMD ["/usr/local/bin/tr-web"]