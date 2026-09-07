# Build the static SPA, then serve it with nginx which also proxies /api
# and /badge to the monitor (same-origin, no CORS needed).
FROM node:26-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Empty VITE_API_URL = same-origin requests; nginx proxies /api to the backend.
ARG VITE_API_URL=
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

FROM nginx:alpine
# Backend the reverse proxy forwards /api and /badge to. Compose service name
# for local dev; set to the monitor's public URL on Coolify. The official
# nginx entrypoint envsubstitutes /etc/nginx/templates/ at container start.
ENV API_BACKEND=http://monitor:8080
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
