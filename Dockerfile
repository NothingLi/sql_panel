# 第一阶段：构建前端
FROM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY web/package*.json ./
RUN npm install
COPY web/ .
ARG VITE_AES_KEY
ENV VITE_AES_KEY=${VITE_AES_KEY}
RUN npm run build


FROM golang:1.26.3 AS builder

WORKDIR /app

COPY server/go.mod server/go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn \
  && go mod download
COPY server/ .

# 复制前端构建产物（覆盖 server/dist 中的占位文件）
COPY --from=frontend-builder /app/frontend/dist ./dist

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/main .


# 运行阶段
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]