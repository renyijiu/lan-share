#!/bin/sh
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec python3 "$project_dir/scripts/build.py" --platform macos --output "${1:-"$project_dir/dist"}"
