#!/bin/sh -x

source ./test.common.sh

NO_LOG_STDOUT=true go test -tags cpunoise ./test/acceptance -count 100 -timeout 20m -failfast > test.out
check_exit_code_and_report

NO_LOG_STDOUT=true go test -tags cpunoise ./services/blockstorage/test -count 100 -timeout 5m -failfast > test.out
check_exit_code_and_report

NO_LOG_STDOUT=true go test -tags cpunoise ./services/blockstorage/sync -count 100 -timeout 5m -failfast > test.out
check_exit_code_and_report
