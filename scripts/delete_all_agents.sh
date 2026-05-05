#!/bin/bash
set -e

DB_PATH="${WINGMAN_DB:-$HOME/.local/share/wingman/wingman.db}"

if [ ! -f "$DB_PATH" ]; then
    echo "Database not found: $DB_PATH"
    exit 1
fi

count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM agents;")
echo "Deleting $count agents..."
sqlite3 "$DB_PATH" "DELETE FROM agents;"
echo "Done."
