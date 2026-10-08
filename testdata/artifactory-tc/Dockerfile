FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/nginx.conf
COPY public/ /usr/share/nginx/html/artifactory/public/
EXPOSE 80
HEALTHCHECK --interval=2s --timeout=2s --retries=15 CMD wget -qO- http://127.0.0.1/healthz || exit 1
