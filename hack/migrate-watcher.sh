#!/usr/bin/env bash
#
# Convert an old-shape Watcher YAML to the new CEL-based shape.
#
# Usage:
#   ./hack/migrate-watcher.sh path/to/watcher.yaml > new.yaml
#   cat watcher.yaml | ./hack/migrate-watcher.sh > new.yaml
#
# Exit codes match the underlying Go tool:
#   0  converted cleanly
#   1  I/O / parse error
#   2  converted with warnings printed to stderr
#      (typically filter.object.custom — Go template cannot be mechanically
#       converted to CEL and must be rewritten by hand)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

exec go run ./hack/migrate-watcher "$@"
