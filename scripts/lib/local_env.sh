# shellcheck shell=bash
# Sourced by scripts/db-reset.sh and scripts/simulate.sh. Not executable on its own.
# Reads one key from config/local.env with the same line grammar as internal/config
# (design "Configuration Surface"). The file is parsed line by line and never executed.
# Compatible with bash 3.2 (macOS default): no associative arrays.

LOCAL_ENV_FILE="config/local.env"

# trim_ws VALUE -> sets TRIMMED to VALUE without leading/trailing whitespace.
trim_ws() {
  local s=$1
  s=${s#"${s%%[![:space:]]*}"}
  s=${s%"${s##*[![:space:]]}"}
  TRIMMED=$s
}

# local_env_get KEY -> prints the value of KEY (last occurrence wins; empty if absent).
# Validates every line, like the Go parser. On a grammar error it prints
# "<file> line <n>: <message>; <hint>" to stderr (never the line content) and returns 1.
# A missing file is not an error; an unreadable one prints "<file>: cannot read file"
# to stderr and returns 1.
local_env_get() {
  local want=$1 line trimmed key val n=0 result=""
  if [[ ! -f $LOCAL_ENV_FILE ]]; then
    return 0
  fi
  # [impl-review-fix] An unreadable file made the redirect below fail while the
  # function still printed an empty value and returned 0, hiding the real cause.
  # Same text as the Go service (internal/config).
  if [[ ! -r $LOCAL_ENV_FILE ]]; then
    echo "$LOCAL_ENV_FILE: cannot read file" >&2
    return 1
  fi
  while IFS= read -r line || [[ -n $line ]]; do
    n=$((n + 1))
    line=${line%$'\r'}
    trim_ws "$line"
    trimmed=$TRIMMED
    if [[ -z $trimmed || $trimmed == \#* ]]; then
      continue
    fi
    if [[ $line != *=* ]]; then
      echo "$LOCAL_ENV_FILE line $n: missing \"=\"; use KEY=VALUE" >&2
      return 1
    fi
    trim_ws "${line%%=*}"
    key=$TRIMMED
    trim_ws "${line#*=}"
    val=$TRIMMED
    case $key in
      TM_DATABASE_URL | TM_LISTEN_ADDR | TM_ALARM_THRESHOLD_C) ;;
      *)
        echo "$LOCAL_ENV_FILE line $n: unknown key; allowed keys: TM_DATABASE_URL, TM_LISTEN_ADDR, TM_ALARM_THRESHOLD_C" >&2
        return 1
        ;;
    esac
    if [[ $val == \"* || $val == \'* ]]; then
      echo "$LOCAL_ENV_FILE line $n: value must not be quoted; remove the quotes" >&2
      return 1
    fi
    if [[ $key == "$want" ]]; then
      result=$val
    fi
  done <"$LOCAL_ENV_FILE"
  printf '%s' "$result"
}
