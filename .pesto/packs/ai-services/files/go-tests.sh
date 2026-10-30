#!/usr/bin/env bash
# go-tests - post_script of every node here that commits: what build.yaml's `build` job runs,
# minus -race. -race needs cgo, and the sandbox image pins CGO_ENABLED=0 with no C compiler, so
# a race is CI's to find. -p 2 bounds the package parallelism to what the pod's cgroup holds.
set -euo pipefail
go build ./...
go test -count=1 -p 2 ./...
