FROM node:18-alpine AS build

ARG UI_VARIANT=generic

WORKDIR /app

# Copy shared source so the variant's relative ../ui-shared/src alias resolves
COPY ui-shared/ ./ui-shared/

# Copy the selected variant
COPY ui-${UI_VARIANT}/ ./ui-${UI_VARIANT}/

WORKDIR /app/ui-${UI_VARIANT}

RUN npm ci

ARG VITE_API_BASE_URL
ARG VITE_WS_BASE_URL
ARG VITE_SIMULATE_PROD=1
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
ENV VITE_WS_BASE_URL=${VITE_WS_BASE_URL}
ENV VITE_SIMULATE_PROD=${VITE_SIMULATE_PROD}
RUN npm run build

FROM nginx:1.25-alpine

COPY deploy/compose/nginx/nginx-https.conf.template /etc/nginx/nginx-https.conf.template
COPY deploy/compose/nginx/nginx-http.conf.template /etc/nginx/nginx-http.conf.template
COPY deploy/compose/nginx/40-render-nginx-conf.sh /docker-entrypoint.d/40-render-nginx-conf.sh
RUN chmod +x /docker-entrypoint.d/40-render-nginx-conf.sh
COPY --from=build /app/ui-${UI_VARIANT}/dist /usr/share/nginx/html
