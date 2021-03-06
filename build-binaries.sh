#!/bin/bash -x

rm -rf _bin

time go build -o _bin/orbs-node -a main.go

time go test -o _bin/e2e.test -a -c ./test/e2e

if [ "$SKIP_DEVTOOLS" == "" ]; then
    time go build -o _bin/gamma-cli -a devtools/gammacli/main/main.go

    time go build -o _bin/gamma-server -a devtools/gammaserver/main/main.go
fi
