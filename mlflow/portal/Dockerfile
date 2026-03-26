# ── Stage 1: Build Svelte frontend ──
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go binary ──
FROM golang:1.23-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN go build -o portal .

# ── Stage 3: Runtime ──
FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=go-builder /app/portal .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

EXPOSE 8080

ENV FRONTEND_DIR=/app/frontend/dist

CMD ["./portal"]
