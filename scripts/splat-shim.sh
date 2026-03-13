#!/bin/bash
# Local development wrapper - runs the built binary
exec "$(dirname "$0")/bin/splat" "$@"
