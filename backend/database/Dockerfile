# Insights: the sync worker and the read API, one image, mode chosen by the command.
FROM node:22.20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY tsconfig.json main.ts ./
COPY lib lib
COPY sync sync
COPY api api
COPY bench bench
RUN npx tsc -p tsconfig.json && npm prune --omit=dev

FROM node:22.20-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app/node_modules node_modules
COPY --from=build /app/dist dist
COPY package.json ./
COPY migrations migrations
USER node
EXPOSE 8090
ENTRYPOINT ["node", "dist/main.js"]
CMD ["all"]
