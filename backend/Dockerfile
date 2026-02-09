FROM golang:1.24-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api

FROM gcr.io/distroless/base-debian12
WORKDIR /app

COPY --from=builder /out/api /app/api

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/api"]
