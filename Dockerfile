# --- фронт ---
FROM node:24-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- бэкенд ---
FROM golang:1.27-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /roadpolice-web .

# --- итоговый образ ---
FROM alpine:3.22
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=backend /roadpolice-web /app/roadpolice-web
COPY --from=frontend /app/dist /app/public
RUN mkdir -p /app/data && chown app:app /app/data
USER app
EXPOSE 8080
ENV RP_ADDR=:8080 RP_BANK_DIR=/app/bank RP_DATA_DIR=/app/data RP_FRONTEND_DIR=/app/public
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/api/health || exit 1
ENTRYPOINT ["/app/roadpolice-web"]
