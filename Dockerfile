FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/ledger-api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/ledger-api /app/ledger-api
COPY db /app/db
EXPOSE 8080
ENTRYPOINT ["/app/ledger-api"]
