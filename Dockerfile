ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go mod tidy
RUN go build -v -o /talent-rodeo ./cmd/web


FROM debian:bookworm
RUN apt-get update -y && apt-get install -y ca-certificates poppler-utils
COPY --from=builder /talent-rodeo /usr/local/bin/

CMD ["/usr/local/bin/talent-rodeo"]