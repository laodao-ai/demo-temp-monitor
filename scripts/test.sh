#!/usr/bin/env bash
# 统一测试入口：scripts/test.sh [unit|integration|all]
# 计数口径与退出码见 openspec 的 design「Test Runner」。本脚本不读 config/local.env。
# 不用 set -e：go test 失败是预期结果，要先记下退出码再解析 JSON 并汇总；
# 失败路径都显式 exit，让每个退出码与 stderr 文案都由本脚本写明。

# 切到仓根（脚本所在目录的上一级），从任何目录调用结果一样。
# 不用外部命令 dirname：缺工具检查要在任何外部命令之前，且受控 PATH 下也要能跑。
script_path="${BASH_SOURCE[0]}"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir="." ;;
esac
cd -- "$script_dir/.." || exit 1

# ${1-all}: default only when no argument is given; an empty string is invalid.
mode="${1-all}"
if [ "$#" -gt 1 ]; then
  mode="__invalid__"
fi
case "$mode" in
  unit|integration|all) ;;
  *)
    echo 'usage: scripts/test.sh [unit|integration|all]' >&2
    exit 2
    ;;
esac

for tool in go jq; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "required tool not found: $tool; install it and make sure it is on PATH" >&2
    exit 1
  fi
done

tmp_dir="$(mktemp -d)" || exit 1
trap 'rm -rf "$tmp_dir"' EXIT

total_pass=0
total_fail=0
total_skip=0
zero_layer=0

# count_filter 与 detail_filter 共用的两个包集合（输入是 slurp 后的事件数组）：
# $pkgs_with_test_fail = 有用例级 fail 的包；$pkgs_fail = 有包级 fail 的包。
pkg_fail_sets='
  (map(select(.Test != null and .Action == "fail") | .Package) | unique) as $pkgs_with_test_fail
  | (map(select(.Test == null and .Action == "fail" and .Package != null) | .Package) | unique) as $pkgs_fail
'

# 从 go test -json 的事件（slurp 成数组）算 "N M K"。
# M = 用例级 fail 数 + 「有包级 fail 但该包没有任何用例级 fail」的包数。
# 带 Test 字段才算用例；没有测试文件的包只产生包级 skip，不计入 K。
count_filter="$pkg_fail_sets"'
  | [
      (map(select(.Test != null and .Action == "pass")) | length),
      ((map(select(.Test != null and .Action == "fail")) | length)
        + (($pkgs_fail - $pkgs_with_test_fail) | length)),
      (map(select(.Test != null and .Action == "skip")) | length)
    ]
  | map(tostring) | join(" ")
'

# 打到 stderr 的内容：失败用例的 output、build-output、贡献了包级 fail 的包的包级 output。
detail_filter="$pkg_fail_sets"'
  | (map(select(.Test != null and .Action == "fail") | .Package + "\u0000" + .Test)) as $failed_tests
  | (($pkgs_fail - $pkgs_with_test_fail)) as $pkgs_pkg_only
  | .[]
  | select(
      (.Action == "build-output")
      or (.Action == "output" and .Test != null and ((.Package + "\u0000" + .Test) as $k | $failed_tests | index($k) != null))
      or (.Action == "output" and .Test == null and .Package != null and (.Package as $p | $pkgs_pkg_only | index($p) != null))
    )
  | .Output
'

jq_parse_failed() {
  echo 'cannot parse go test -json output; see the jq error above' >&2
  exit 1
}

# run_layer <层名> <go test 额外参数...>
# 结果累加到 total_*；jq 解析失败时直接 exit 1。
run_layer() {
  local layer="$1"
  shift
  local json="$tmp_dir/$layer.json"
  local status=0

  # go test 的 stderr 不进 jq，直接透传。
  go test -json -count=1 "$@" >"$json" || status=$?

  local counts
  counts="$(jq -s -r "$count_filter" "$json")" || jq_parse_failed
  jq -s -j "$detail_filter" "$json" >&2 || jq_parse_failed

  local n m k
  read -r n m k <<<"$counts"

  # go test 非零退出而 M = 0：进程被杀等情形，不能算绿。
  if [ "$status" -ne 0 ] && [ "$m" -eq 0 ]; then
    m=1
    echo "go test exited with status $status but reported no failure; see the go test output above" >&2
  fi

  if [ $((n + m)) -eq 0 ]; then
    echo "no tests executed in $layer layer; check the integration build tag and the TestIntegration name prefix" >&2
    zero_layer=1
  fi

  total_pass=$((total_pass + n))
  total_fail=$((total_fail + m))
  total_skip=$((total_skip + k))
}

if [ "$mode" = "unit" ] || [ "$mode" = "all" ]; then
  run_layer unit -race ./...
fi
if [ "$mode" = "integration" ] || [ "$mode" = "all" ]; then
  run_layer integration -p 1 -tags integration -run '^TestIntegration' ./...
fi

echo "$total_pass passed, $total_fail failed, $total_skip skipped ($((total_pass + total_fail + total_skip)) total)"

if [ "$zero_layer" -eq 1 ]; then
  exit 1
fi
if [ "$total_fail" -gt 125 ]; then
  exit 125
fi
exit "$total_fail"
