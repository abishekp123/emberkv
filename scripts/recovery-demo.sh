#!/usr/bin/env bash
set -euo pipefail

dir="${1:-/tmp/emberkv-demo}"
rm -rf "$dir"
mkdir -p "$dir"

go run ./cmd/emberkv -dir "$dir" -cmd set -key alpha -value one
go run ./cmd/emberkv -dir "$dir" -cmd set -key beta -value two
printf '\nBefore restart:\n'
go run ./cmd/emberkv -dir "$dir" -cmd get -key alpha

# Simulate a crash by truncating the log after the last valid record.
python3 - <<'PY' "$dir"
import os, sys
path = os.path.join(sys.argv[1], 'emberkv.log')
with open(path, 'rb') as f:
    data = f.read()
# keep the trailing byte but cut off the full record tail
with open(path, 'wb') as f:
    f.write(data[:-5])
PY

printf '\nAfter simulated crash and reopen:\n'
go run ./cmd/emberkv -dir "$dir" -cmd get -key alpha
