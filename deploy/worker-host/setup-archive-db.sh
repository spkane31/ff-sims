#!/usr/bin/env bash
set -euo pipefail

PG_MAJOR="16"
DB_NAME="ff_sims_archive"
DB_USER="ff_sims_archive"
PASSWORD_FILE="/etc/ff-sims-archive-db.pass"

# db_url_for is a pure string builder — kept separate from the psql/apt calls
# below so it's the one piece of this script that's easy to reason about
# without root.
db_url_for() {
  local user="$1" password="$2" db="$3"
  echo "postgres://${user}:${password}@localhost:5432/${db}?sslmode=disable"
}

pg_already_installed() {
  command -v psql &>/dev/null && psql --version | grep -q " ${PG_MAJOR}\."
}

install_postgres() {
  if pg_already_installed; then
    echo "PostgreSQL ${PG_MAJOR} already installed"
    return
  fi
  echo "Installing PostgreSQL ${PG_MAJOR} from the PGDG apt repo"
  apt-get update -y
  apt-get install -y curl ca-certificates gnupg
  install -d /usr/share/postgresql-common/pgdg
  curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
    -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc
  # shellcheck disable=SC1091
  . /etc/os-release
  echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt ${VERSION_CODENAME}-pgdg main" \
    > /etc/apt/sources.list.d/pgdg.list
  apt-get update -y
  apt-get install -y "postgresql-${PG_MAJOR}"
}

ensure_password() {
  if [[ ! -f "$PASSWORD_FILE" ]]; then
    openssl rand -hex 24 > "$PASSWORD_FILE"
    chmod 600 "$PASSWORD_FILE"
  fi
  cat "$PASSWORD_FILE"
}

role_exists() {
  sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1
}

db_exists() {
  sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1
}

ensure_role_and_db() {
  local password="$1"
  if role_exists; then
    echo "Role ${DB_USER} already exists"
    sudo -u postgres psql -c "ALTER ROLE ${DB_USER} WITH PASSWORD '${password}'" >/dev/null
  else
    echo "Creating role ${DB_USER}"
    sudo -u postgres psql -c "CREATE ROLE ${DB_USER} WITH LOGIN PASSWORD '${password}'" >/dev/null
  fi

  if db_exists; then
    echo "Database ${DB_NAME} already exists"
  else
    echo "Creating database ${DB_NAME}"
    sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER}" >/dev/null
  fi
}

print_summary() {
  local password="$1" url
  url="$(db_url_for "$DB_USER" "$password" "$DB_NAME")"
  cat <<EOF

Archive DB ready.

Add this to /etc/ff-sims-worker.env, then run:
  sudo systemctl restart ff-sims-worker

ARCHIVE_DATABASE_URL=${url}

The Debian/Ubuntu PGDG package's default postgresql.conf already binds
listen_addresses='localhost' — no firewall changes needed since the worker
connects from this same machine.
EOF
}

main() {
  install_postgres
  local password
  password="$(ensure_password)"
  ensure_role_and_db "$password"
  print_summary "$password"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
