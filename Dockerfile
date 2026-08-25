FROM node:24-bookworm-slim AS web
WORKDIR /web
COPY web/package.json ./
RUN npm install --registry=https://registry.npmmirror.com
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS api
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
COPY server/ ./
RUN go build -o /out/api ./cmd/api

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=api /out/api /app/api
COPY --from=web /web/dist /app/web/dist
ENV WEB_DIR=/app/web/dist
ENV PORT=8787
EXPOSE 8787
CMD ["/app/api"]
