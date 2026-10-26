# syntax=docker/dockerfile:1
FROM node:22-bookworm-slim AS build
RUN corepack enable && corepack prepare pnpm@10.11.1 --activate
WORKDIR /app
COPY medusa-store/ ./
RUN pnpm install --filter @dtc/storefront... --frozen-lockfile
ARG NEXT_PUBLIC_MEDUSA_BACKEND_URL
ARG NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY
ARG NEXT_PUBLIC_BASE_URL
ARG NEXT_PUBLIC_DEFAULT_REGION=dk
ARG NEXT_PUBLIC_STRIPE_KEY
ARG NEXT_PUBLIC_MEDUSA_PAYMENTS_PUBLISHABLE_KEY
ARG NEXT_PUBLIC_MEDUSA_PAYMENTS_ACCOUNT_ID
ENV NEXT_TELEMETRY_DISABLED=1
RUN test -n "$NEXT_PUBLIC_MEDUSA_BACKEND_URL" && test -n "$NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY" \
    && test -n "$NEXT_PUBLIC_BASE_URL" && pnpm --filter @dtc/storefront build

FROM node:22-bookworm-slim AS runtime
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1
WORKDIR /app/apps/storefront
COPY --from=build /app/node_modules /app/node_modules
COPY --from=build /app/apps/storefront/node_modules ./node_modules
COPY --from=build --chown=node:node /app/apps/storefront/.next ./.next
COPY --from=build /app/apps/storefront/public ./public
COPY --from=build /app/apps/storefront/next.config.js /app/apps/storefront/check-env-variables.js /app/apps/storefront/package.json ./
USER node
EXPOSE 8000
CMD ["node", "node_modules/next/dist/bin/next", "start", "-H", "0.0.0.0", "-p", "8000"]
