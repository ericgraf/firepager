FROM node:22-alpine

ENV NODE_ENV=production \
    BRIDGE_HOST=0.0.0.0 \
    BRIDGE_PORT=8787

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npm cache clean --force
COPY --chown=node:node server.js ./server.js
RUN mkdir -p /app/data/auth && chown -R node:node /app/data

USER node
EXPOSE 8787
VOLUME ["/app/data/auth"]
CMD ["node", "server.js"]
