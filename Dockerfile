# Dockerfile

# ---- build stage ----
FROM golang:1.22 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /server main.go

# ---- run stage ----
FROM alpine:3.20
WORKDIR /app
# 預先建立資料夾（Render 會把 Disk 掛上 /data；這步只是保險）
RUN mkdir -p /data/uploads
ENV DATA_DIR=/data
ENV PORT=8080
COPY --from=build /server /app/server
EXPOSE 8080
CMD ["/app/server"]
