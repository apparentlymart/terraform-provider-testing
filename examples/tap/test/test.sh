
echo "1..1"
if [ "$1" != "hello" ]; then
    echo "# got $1 but want hello"
    echo "not ok 1 greeting is correct"
    exit 0
fi
echo "ok 1 greeting is correct"
