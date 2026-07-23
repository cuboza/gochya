#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cargo build --release -p gochya-core --manifest-path "$project_root/Cargo.toml"

cd "$project_root/server"

go mod verify

unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  echo "Go files need formatting:"
  echo "$unformatted"
  exit 1
fi

go vet -tags gochya_core ./...
CGO_ENABLED=1 go test -race -tags gochya_core ./...
