# syntax=docker/dockerfile:1
FROM golang:1.23 as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN GOOS=linux GOARCH=amd64 go build -o server ./cmd/server
RUN chmod +x /app/server

# ---

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /app/server /app/
COPY .env .env

EXPOSE 8081

CMD ["/app/server"] 