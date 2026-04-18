# ── Stage 1: Build Next.js dashboard ─────────────────────────────────────────
FROM node:20-alpine AS ui-builder
WORKDIR /ui

COPY ui/package.json ui/package-lock.json ./
RUN npm ci --ignore-scripts

COPY ui/ ./
# Empty string → browser uses relative URLs → Next.js rewrites proxy to Go
ENV NEXT_PUBLIC_ARGUS_SERVER=
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

RUN npm install -g pm2 && apk add --no-cache curl

WORKDIR /app

COPY --from=server-builder /out/argus ./argus
COPY --from=ui-builder /ui/.next/standalone ./ui/
COPY --from=ui-builder /ui/.next/static ./ui/.next/static
COPY --from=ui-builder /ui/public ./ui/public
COPY deploy/ecosystem.config.js ./ecosystem.config.js

ENV ARGUS_ADDR=:4000
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

EXPOSE 4000 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD curl -f http://localhost:3000/healthz || exit 1

CMD ["pm2-runtime", "ecosystem.config.js"]
