FROM node:24-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY fixtures/ fixtures/
RUN CGO_ENABLED=0 go build -trimpath -o /evalgate ./cmd/evalgate

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=backend /evalgate /usr/local/bin/evalgate
COPY --from=frontend /src/web/dist ./web/dist
USER 65532:65532
ENV PORT=8080
EXPOSE 8080
CMD ["evalgate", "serve"]
