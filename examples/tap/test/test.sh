
set +euxo pipefail

num=1

function plan() {
    printf "1..%d\n" "$1"
}

function equal() {
  local got="$1"
  local want="$2"
  local name="$3"
  local this_num="$num"
  let num++
  if [ "$got" != "$want" ]; then
    echo "# Got:  $got"
    echo "# Want: $want"
    echo "not ok $this_num $name"
  else
    echo "ok $this_num $name"
  fi
}

plan 2
equal "$1" "hello" "greeting is correct"
equal "$FOO" "bar" "environment variable FOO is correct"
