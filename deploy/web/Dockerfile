FROM node:22-alpine AS build

WORKDIR /src

ARG VITE_AMAP_JS_KEY=
ARG VITE_AMAP_SECURITY_CODE=
ENV VITE_AMAP_JS_KEY=$VITE_AMAP_JS_KEY
ENV VITE_AMAP_SECURITY_CODE=$VITE_AMAP_SECURITY_CODE

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM caddy:2-alpine

COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /src/dist /srv

EXPOSE 80
