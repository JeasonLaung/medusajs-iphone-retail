# syntax=docker/dockerfile:1
FROM node:22-bookworm-slim AS build
RUN corepack enable && corepack prepare pnpm@10.11.1 --activate
WORKDIR /app
COPY medusa-store/ ./
RUN pnpm install --filter @dtc/backend... --frozen-lockfile
RUN pnpm --filter @dtc/backend build

FROM node:22-bookworm-slim AS runtime
ENV NODE_ENV=production PORT=9000 HOST=0.0.0.0 MEDUSA_TELEMETRY_DISABLED=true
WORKDIR /app/apps/backend
# Preserve pnpm's relative symlinks and the exact dependencies used to build.
COPY --from=build /app/node_modules /app/node_modules
COPY --from=build /app/apps/backend/node_modules ./node_modules
COPY --from=build /app/apps/backend/.medusa/server ./
COPY --from=build /app/apps/backend/static /opt/static-assets
COPY deploy/backend-entrypoint.sh /usr/local/bin/backend-entrypoint
RUN mkdir -p static .private-files && chown -R node:node static .private-files \
    && chmod +x /usr/local/bin/backend-entrypoint
USER node
EXPOSE 9000
ENTRYPOINT ["backend-entrypoint"]
CMD ["node", "node_modules/@medusajs/cli/cli.js", "start"]
