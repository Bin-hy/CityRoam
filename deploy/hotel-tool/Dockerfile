FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund
COPY server.mjs ./

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production \
    PORT=8090 \
    FLYAI_BIN=/app/node_modules/.bin/flyai
COPY --from=build --chown=node:node /app /app
USER node
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:8090/healthz >/dev/null || exit 1
CMD ["node", "server.mjs"]
