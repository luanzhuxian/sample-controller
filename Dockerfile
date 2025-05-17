FROM golang:1.24 AS builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o sample-controller .

# 使用更小的基础镜像运行
FROM alpine:latest

WORKDIR /root/
COPY --from=builder /app/sample-controller .

ENTRYPOINT ["./sample-controller"]