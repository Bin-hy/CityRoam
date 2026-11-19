#!/bin/sh
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"

BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

mkdir -p "$BACKUP_DIR"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="$BACKUP_DIR/trippilot-$stamp.dump"

pg_dump --format=custom --no-owner --no-privileges --file="$target" "$DATABASE_URL"
pg_restore --list "$target" >/dev/null

find "$BACKUP_DIR" -maxdepth 1 -type f -name 'trippilot-*.dump' -mtime +"$RETENTION_DAYS" -delete

echo "backup written: $target"
