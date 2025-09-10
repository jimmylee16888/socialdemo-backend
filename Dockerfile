# Dockerfile
FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server .

FROM gcr.io/distroless/base-debian12
WORKDIR /app
ENV DATA_DIR=/data
ENV PORT=8080
COPY --from=builder /app/server /app/server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
