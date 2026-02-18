FROM node:18-alpine AS build

WORKDIR /app
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
ARG VITE_API_BASE_URL
ARG VITE_WS_BASE_URL
ARG VITE_SIMULATE_PROD=1
ARG VITE_MAINTENANCE_TOKEN
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
ENV VITE_WS_BASE_URL=${VITE_WS_BASE_URL}
ENV VITE_SIMULATE_PROD=${VITE_SIMULATE_PROD}
ENV VITE_MAINTENANCE_TOKEN=${VITE_MAINTENANCE_TOKEN}
RUN npm run build

FROM nginx:1.25-alpine

COPY deploy/compose/nginx/nginx-https.conf.template /etc/nginx/nginx-https.conf.template
COPY deploy/compose/nginx/nginx-http.conf.template /etc/nginx/nginx-http.conf.template
COPY deploy/compose/nginx/40-render-nginx-conf.sh /docker-entrypoint.d/40-render-nginx-conf.sh
RUN chmod +x /docker-entrypoint.d/40-render-nginx-conf.sh
COPY --from=build /app/dist /usr/share/nginx/html
