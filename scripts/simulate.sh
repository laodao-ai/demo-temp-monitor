#!/usr/bin/env bash
# Simulate three devices reporting temperatures over HTTP. sim-03 is over the
# default 50.0 threshold every round.
# Exit codes: 0 ok; 1 request failed or bad config/local.env; 2 usage.
set -u
export LC_ALL=C

# Run from the repo root so the result does not depend on the caller's directory.
cd "$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)" || exit 1
# shellcheck source=scripts/lib/local_env.sh
source scripts/lib/local_env.sh

usage() {
  echo "usage: scripts/simulate.sh [rounds]  (rounds: positive integer, default 10)" >&2
  exit 2
}

if [[ $# -gt 1 ]]; then
  usage
fi
# ${1-10}: default only when no argument is given; an empty string is a usage error.
rounds=${1-10}
if [[ ! $rounds =~ ^[0-9]+$ ]] || ((10#$rounds < 1)); then
  usage
fi
rounds=$((10#$rounds))

# Same source order as the service: non-empty env var > config/local.env > default.
addr=${TM_LISTEN_ADDR:-}
if [[ -z $addr ]]; then
  if ! addr=$(local_env_get TM_LISTEN_ADDR); then
    exit 1
  fi
fi
addr=${addr:-127.0.0.1:8080}
if [[ $addr == :* ]]; then
  addr="127.0.0.1$addr"
fi
base="http://$addr"

body_file=$(mktemp) || exit 1
trap 'rm -f "$body_file"' EXIT

# request METHOD PATH JSON EXPECTED_CODES...
# Prints the request and response and exits 1 when the status is not expected.
# Sets STATUS to the HTTP status code.
request() {
  local method=$1 path=$2 data=$3 code rc ok=0 expected
  shift 3
  # --noproxy: the service runs on this machine only; an http_proxy in the
  # environment must not route these requests away from it.
  code=$(curl -sS --noproxy '*' --max-time 5 -o "$body_file" -w '%{http_code}' \
    -X "$method" -H 'Content-Type: application/json' -d "$data" "$base$path" 2>/dev/null)
  rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "request failed: $method $base$path -> curl error $rc; start the service first, or check TM_LISTEN_ADDR" >&2
    echo "request body: $data" >&2
    exit 1
  fi
  for expected in "$@"; do
    if [[ $code == "$expected" ]]; then
      ok=1
    fi
  done
  if [[ $ok -ne 1 ]]; then
    echo "request failed: $method $base$path -> $code; start the service first, or check TM_LISTEN_ADDR" >&2
    echo "request body: $data" >&2
    echo "response body: $(cat "$body_file")" >&2
    exit 1
  fi
  STATUS=$code
}

# tenths MIN MAX -> random value in [MIN/10, MAX/10] with one decimal place.
tenths() {
  local v=$(($1 + RANDOM % ($2 - $1 + 1)))
  printf '%d.%d' $((v / 10)) $((v % 10))
}

devices=(sim-01 sim-02 sim-03)

for d in "${devices[@]}"; do
  request POST /devices "{\"id\":\"$d\"}" 201 409
  if [[ $STATUS == 409 ]]; then
    echo "$d already registered, continuing"
  else
    echo "registered $d"
  fi
done

round=1
while ((round <= rounds)); do
  for d in "${devices[@]}"; do
    if [[ $d == sim-03 ]]; then
      t=$(tenths 505 600)
    else
      t=$(tenths 180 350)
    fi
    request POST "/devices/$d/readings" "{\"temperature_c\":$t}" 201
    echo "round $round: $d $t"
  done
  if ((round < rounds)); then
    sleep 0.2
  fi
  round=$((round + 1))
done
