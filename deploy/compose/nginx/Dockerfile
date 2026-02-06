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

COPY deploy/compose/nginx/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /app/dist /usr/share/nginx/html
