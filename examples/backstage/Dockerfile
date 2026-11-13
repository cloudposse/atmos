FROM python:3.12-slim AS techdocs
RUN pip install --no-cache-dir mkdocs-techdocs-core==1.7.1
COPY catalog/techdocs /docs
RUN mkdocs build --strict --config-file /docs/mkdocs.yml --site-dir /site

# Development image: keep the generated Backstage app out of the example source.
FROM node:22-bookworm-slim

ARG ATMOS_VERSION=1.234.0
ARG BACKSTAGE_CREATE_APP_VERSION=0.9.3
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git python3 g++ make libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g yarn --force \
    && curl -fsSL "https://github.com/cloudposse/atmos/releases/download/v${ATMOS_VERSION}/atmos_${ATMOS_VERSION}_linux_${TARGETARCH}" -o /usr/local/bin/atmos \
    && chmod +x /usr/local/bin/atmos \
    && atmos version

RUN mkdir -p /app && chown node:node /app
USER node
WORKDIR /app
ENV BACKSTAGE_APP_NAME=portal
RUN npx --yes @backstage/create-app@${BACKSTAGE_CREATE_APP_VERSION} --skip-install
WORKDIR /app/portal
COPY backstage/configure.mjs /tmp/configure.mjs
RUN node /tmp/configure.mjs && yarn install
COPY --chown=node:node backstage/atmos.ts packages/backend/src/atmos.ts
COPY --chown=node:node backstage/request-service.ts packages/backend/src/request-service.ts
COPY --chown=node:node backstage/app-config.yaml app-config.yaml
COPY --chown=node:node backstage/GiteaActions.tsx packages/app/src/GiteaActions.tsx
COPY --chown=node:node backstage/geist.css packages/app/src/geist.css
COPY --chown=node:node backstage/GeistDocs.tsx packages/app/src/GeistDocs.tsx
COPY --chown=node:node backstage/themes.tsx packages/app/src/themes.tsx
COPY --chown=node:node backstage/PortalLogo.tsx packages/app/src/PortalLogo.tsx
COPY --chown=node:node backstage/App.tsx packages/app/src/App.tsx
COPY --chown=node:node backstage/portalModules.tsx packages/app/src/portalModules.tsx
COPY --chown=node:node backstage/PortalNav.tsx packages/app/src/PortalNav.tsx
COPY --chown=node:node backstage/PortalCatalog.tsx packages/app/src/PortalCatalog.tsx
RUN yarn tsc
COPY catalog /demo/catalog
COPY templates /demo/templates
COPY atmos.yaml /demo/atmos.yaml
COPY stacks /demo/stacks
COPY components /demo/components
COPY scripts /demo/scripts
COPY scaffolds /demo/scaffolds
COPY --from=techdocs /site /demo/techdocs-site/default/component/payments

ENV NODE_ENV=development
ENV ATMOS_TELEMETRY_ENABLED=false
EXPOSE 3000 7007
CMD ["yarn", "start"]
