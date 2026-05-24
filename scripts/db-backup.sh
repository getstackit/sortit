#!/bin/sh

set -eu

find_pg_dump() {
	if command -v pg_dump >/dev/null 2>&1; then
		command -v pg_dump
		return 0
	fi

	for candidate in /opt/homebrew/opt/libpq/bin/pg_dump /usr/local/opt/libpq/bin/pg_dump; do
		if [ -x "$candidate" ]; then
			printf '%s\n' "$candidate"
			return 0
		fi
	done

	return 1
}

if [ -z "${SORTIT_DATABASE_URL:-}" ]; then
	echo "SORTIT_DATABASE_URL is required" >&2
	exit 1
fi

pg_dump_bin=$(find_pg_dump || true)
if [ -z "$pg_dump_bin" ]; then
	echo "pg_dump not found in PATH or common Homebrew libpq locations" >&2
	exit 1
fi

backup_dir="${SORTIT_DB_BACKUP_DIR:-backups/postgres}"
mkdir -p "$backup_dir"

timestamp=$(date -u +"%Y%m%dT%H%M%SZ")
filename="$backup_dir/sortit-$timestamp.dump"

"$pg_dump_bin" \
	--format=custom \
	--file="$filename" \
	"$SORTIT_DATABASE_URL"

printf 'Wrote %s\n' "$filename"
