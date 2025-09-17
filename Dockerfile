# ---- build stage ----
FROM golang:1.24-alpine AS build
WORKDIR /app
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=0 GOOS=linux
# ★ 關鍵：對「目前目錄」建置，不要寫 main.go
RUN go build -ldflags="-s -w" -o /app/server .

# ---- run stage ----
FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates && update-ca-certificates

# 準備資料夾（Render 的 Disk 會掛到 /data）
RUN mkdir -p /data/uploads
ENV DATA_DIR=/data

COPY --from=build /app/server /app/server
EXPOSE 8080
CMD ["/app/server"]
