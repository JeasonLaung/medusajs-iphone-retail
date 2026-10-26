#!/bin/sh
set -eu
# Refresh versioned assets while keeping uploaded photos and private files.
cp -R /opt/static-assets/. ./static/
exec "$@"
