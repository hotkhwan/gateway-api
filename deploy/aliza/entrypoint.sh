#!/bin/sh
# Set ulimit before anything else
ulimit -n 65535
echo "✅ ulimit set to: $(ulimit -n)"

# Start a shell (interactive debug environment)
exec sh