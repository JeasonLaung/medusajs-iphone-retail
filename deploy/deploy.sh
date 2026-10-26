#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
export DEPLOY_ROOT=${1:?Deployment root required}
prefix=${2:?Image prefix required}
sha=${3:?Commit SHA required}
backend=${4:?Backend flag required}
storefront=${5:?Storefront flag required}
[[ "$DEPLOY_ROOT" =~ ^/[a-zA-Z0-9_/-]+$ && "$DEPLOY_ROOT" != / ]]
[[ "$prefix" =~ ^ghcr.io/[a-z0-9._/-]+$ && "$sha" =~ ^[a-f0-9]{40}$ ]]
[[ "$backend" =~ ^(true|false)$ && "$storefront" =~ ^(true|false)$ ]]
release=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$DEPLOY_ROOT"
exec 9> .deploy.lock
flock -w 1800 9
for file in .env backend.env storefront.env; do
  [[ -f "$file" ]] || { echo "Missing server configuration: $file" >&2; exit 1; }
done
if [[ -f .images.env ]]; then
  # Written only by this script; contains validated image references.
  source .images.env
else
  [[ "$backend" == true && "$storefront" == true ]] || {
    echo 'First deployment must select all services.' >&2; exit 1;
  }
fi
export BACKEND_IMAGE=${BACKEND_IMAGE:-$prefix/backend:$sha}
export STOREFRONT_IMAGE=${STOREFRONT_IMAGE:-$prefix/storefront:$sha}
compose() { docker compose --env-file "$DEPLOY_ROOT/.env" -f "$release/compose.production.yaml" "$@"; }
save_images() {
  printf 'BACKEND_IMAGE=%s\nSTOREFRONT_IMAGE=%s\n' "$BACKEND_IMAGE" "$STOREFRONT_IMAGE" > .images.env.tmp
  mv .images.env.tmp .images.env
  cp "$release/compose.production.yaml" compose.production.yaml
}
compose config --quiet
# Unchanged infrastructure is reused. Never remove volumes or run compose down.
compose up -d --wait --wait-timeout 120 postgres redis
deploy_service() {
  local service=$1 variable=$2 old_image=$3 next_image="$prefix/$1:$sha"
  local previous_container
  previous_container=$(compose ps -a -q "$service")
  export "$variable=$next_image"
  compose pull "$service"
  if [[ "$service" == backend ]]; then
    # One migration job, before replacing the running backend. Failure stops deployment.
    # MikroORM creates migration directories inside installed provider packages.
    # Only the short-lived migrator needs root; the application remains user node.
    compose run --rm --no-deps --user 0:0 backend node node_modules/@medusajs/cli/cli.js db:migrate
  fi
  if ! compose up -d --no-deps --wait --wait-timeout 240 "$service"; then
    echo "Health check failed for $service; restoring previous image if available." >&2
    if [[ -n "$previous_container" ]]; then
      export "$variable=$old_image"
      compose up -d --no-deps --wait --wait-timeout 240 "$service" || {
        echo "Previous $service image also failed health checks; inspect service logs." >&2;
      }
    fi
    # Database migrations are never automatically reversed.
    exit 1
  fi
  printf '%s\n' "$old_image" > ".$service.previous-image"
  save_images
}
if [[ "$backend" == true ]]; then deploy_service backend BACKEND_IMAGE "$BACKEND_IMAGE"; fi
if [[ "$storefront" == true ]]; then deploy_service storefront STOREFRONT_IMAGE "$STOREFRONT_IMAGE"; fi
compose ps
