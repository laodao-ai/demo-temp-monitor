#!/usr/bin/env bash
# Rebuild the demo database from scratch in one transaction.
# Exit codes: 0 ok; 1 missing TM_DATABASE_URL or bad config/local.env; 2 usage;
# 3 database name mismatch; 4 psql failed.
set -u

# Run from the repo root so the result does not depend on the caller's directory.
cd "$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)" || exit 1
# shellcheck source=scripts/lib/local_env.sh
source scripts/lib/local_env.sh

readonly DEMO_DB="temp_monitor_demo"
readonly TUNNEL_HINT="check the SSH tunnel and that the URL uses the session pool port 7432"

if [[ $# -gt 0 ]]; then
  echo "usage: scripts/db-reset.sh (takes no arguments)" >&2
  exit 2
fi

# Environment variable (non-empty) wins; otherwise read the one key from the file.
url=${TM_DATABASE_URL:-}
if [[ -z $url ]]; then
  if ! url=$(local_env_get TM_DATABASE_URL); then
    exit 1
  fi
fi
if [[ -z $url ]]; then
  echo "TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in" >&2
  exit 1
fi
TM_DATABASE_URL=$url
export PGCONNECT_TIMEOUT=5

# Password = text between the first ":" after the scheme and the last "@".
# Empty when it cannot be extracted; then no psql error text is ever printed.
password=""
rest=${TM_DATABASE_URL#*://}
if [[ $rest == *:* ]]; then
  after_colon=${rest#*:}
  if [[ $after_colon == *@* ]]; then
    password=${after_colon%@*}
  fi
fi

# [impl-review-fix] README tells users to URL-encode special characters, so the
# URL holds "p%40ss" while psql may print "p@ss". Also mask the decoded form.
# Skipped when the password has a backslash: %b would misread it as an escape.
password_decoded=""
if [[ -n $password && $password == *%* && $password != *\\* ]]; then
  printf -v password_decoded '%b' "${password//%/\\x}"
  if [[ $password_decoded == "$password" ]]; then
    password_decoded=""
  fi
fi

# Replace the whole URL and the password with *** (literal match, quoted patterns).
redact() {
  local s=$1
  s=${s//"$TM_DATABASE_URL"/***}
  if [[ -n $password ]]; then
    s=${s//"$password"/***}
  fi
  if [[ -n $password_decoded ]]; then
    s=${s//"$password_decoded"/***}
  fi
  printf '%s' "$s"
}

# [impl-review-fix] Without psql the guard query below would fail silently and
# the "check the SSH tunnel" hint would send the user the wrong way.
if ! command -v psql >/dev/null 2>&1; then
  echo "required tool not found: psql; install it and make sure it is on PATH" >&2
  exit 4
fi

# Guard: look at the database actually connected to, not at the URL text.
# Connection-phase errors are never printed: a malformed password can leak into
# the host name the driver reports, which redaction cannot catch.
if ! actual_db=$(psql "$TM_DATABASE_URL" -XAtqc 'select current_database()' 2>/dev/null); then
  echo "cannot connect to database; $TUNNEL_HINT" >&2
  exit 4
fi
if [[ $actual_db != "$DEMO_DB" ]]; then
  echo "refusing to reset: only $DEMO_DB is allowed, connected database is $actual_db; check TM_DATABASE_URL" >&2
  exit 3
fi

# Rebuild: drop -> migrations (lexicographic order, LC_ALL=C) -> seed, one transaction.
export LC_ALL=C
files=(-f db/drop.sql)
for f in db/migrations/*.sql; do
  files+=(-f "$f")
done
files+=(-f db/seed.sql)

if ! err=$(psql "$TM_DATABASE_URL" -Xq -v ON_ERROR_STOP=1 --single-transaction \
  -c "SET lock_timeout = '5s'" "${files[@]}" 2>&1 >/dev/null); then
  if [[ -n $password ]]; then
    echo "psql failed: $(redact "$err"); nothing was changed; check the SSH tunnel, then the SQL files under db/" >&2
  else
    echo "psql failed; nothing was changed; check the SSH tunnel, then the SQL files under db/" >&2
  fi
  exit 4
fi
echo "rebuilt database $DEMO_DB"
