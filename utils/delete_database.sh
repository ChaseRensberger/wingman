#!/bin/bash
set -e

DB_PATH="${WINGMAN_DB:-$HOME/.local/share/wingman/wingman.db}"

if [ ! -f "$DB_PATH" ]; then
    echo "Database not found: $DB_PATH"
    exit 1
fi

rm "$DB_PATH"
echo "Deleted database: $DB_PATH"
