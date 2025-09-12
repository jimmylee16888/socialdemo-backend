# ---- build stage ----
FROM golang:1.24-alpine AS build
WORKDIR /app

# 抓 modules 需要 git；https 憑證也一併裝上
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=0 GOOS=linux
# 體積更小
RUN go build -ldflags="-s -w" -o /server main.go

# ---- run stage ----
FROM alpine:3.20
WORKDIR /app

# HTTPS 憑證（必要，否則 VerifyIDToken 取 JWKS/Google API 會失敗）
RUN apk add --no-cache ca-certificates && update-ca-certificates

# 預先建立資料夾（Render 的 Disk 會掛在 /data，上線時仍會覆蓋）
RUN mkdir -p /data/uploads
ENV DATA_DIR=/data

# 不要硬編 PORT，Render 會自動注入
# ENV PORT=8080

COPY --from=build /server /app/server
EXPOSE 8080

CMD ["/app/server"]
