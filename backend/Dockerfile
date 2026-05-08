FROM golang:1.25-alpine AS builder

WORKDIR /app

ENV CGO_ENABLED=0
ENV GOOS=linux

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG APP_VERSION=dev
RUN go build -ldflags="-w -s -X main.version=${APP_VERSION}" -o server ./cmd/server

FROM alpine:3.19

WORKDIR /root/
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/server .

EXPOSE 8080
CMD ["./server"]
