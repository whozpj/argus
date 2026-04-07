# ── Stage 1: Build Next.js dashboard ─────────────────────────────────────────
FROM node:20-alpine AS ui-builder
WORKDIR /ui

COPY ui/package.json ui/package-lock.json ./
RUN npm ci --ignore-scripts

COPY ui/ ./
# Point the dashboard at the server running in the same container
ENV NEXT_PUBLIC_ARGUS_SERVER=http://localhost:4000
RUN npm run build

# ── Stage 2: Build Go server ──────────────────────────────────────────────────
FROM golang:1.26-alpine AS server-builder
WORKDIR /server

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/argus ./cmd/main.go

# ── Stage 3: Runtime image ────────────────────────────────────────────────────
FROM node:20-alpine AS runtime

# Install a minimal process supervisor so both server + Next.js run in one container
RUN npm install -g pm2

WORKDIR /app

# Copy Go binary
COPY --from=server-builder /out/argus ./argus

# Copy Next.js standalone output
COPY --from=ui-builder /ui/.next/standalone ./ui/
COPY --from=ui-builder /ui/.next/static ./ui/.next/static
COPY --from=ui-builder /ui/public ./ui/public

# pm2 ecosystem file
COPY deploy/ecosystem.config.js ./ecosystem.config.js

ENV ARGUS_ADDR=:4000
ENV ARGUS_DB_PATH=/data/argus.db
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

EXPOSE 4000 3000

VOLUME ["/data"]

CMD ["pm2-runtime", "ecosystem.config.js"]
