#!/bin/bash

# Demo script for Log Aggregation System - Phase 1

set -e

echo "=========================================="
echo "Log Aggregation System - Phase 1 Demo"
echo "=========================================="
echo ""

# Clean up from previous runs
rm -rf /tmp/demo-logs /tmp/demo-checkpoints
mkdir -p /tmp/demo-logs

echo "1. Creating demo log file..."
LOG_FILE="/tmp/demo-logs/app.log"
echo "$(date) - Application started" > "$LOG_FILE"
echo "$(date) - Processing request 1" >> "$LOG_FILE"
echo "$(date) - Processing request 2" >> "$LOG_FILE"
echo ""

echo "2. Creating configuration..."
cat > /tmp/demo-config.yaml <<EOF
inputs:
  files:
    - paths:
        - /tmp/demo-logs/app.log
      checkpoint_path: /tmp/demo-checkpoints
      checkpoint_interval: 2s

logging:
  level: info
  format: console

output:
  type: stdout
EOF

echo "3. Starting log aggregator in background..."
./bin/logaggregator -config /tmp/demo-config.yaml &
PID=$!
sleep 2

echo ""
echo "4. Adding new log lines (should be tailed)..."
echo "$(date) - Processing request 3" >> "$LOG_FILE"
echo "$(date) - Processing request 4" >> "$LOG_FILE"
sleep 1

echo ""
echo "5. Simulating log rotation..."
mv "$LOG_FILE" "$LOG_FILE.1"
echo "$(date) - New file after rotation" > "$LOG_FILE"
echo "$(date) - Processing request 5" >> "$LOG_FILE"
sleep 2

echo ""
echo "6. Checking checkpoints..."
if [ -f /tmp/demo-checkpoints/positions.json ]; then
    echo "✓ Checkpoint file created:"
    cat /tmp/demo-checkpoints/positions.json
else
    echo "✗ Checkpoint file not found"
fi

echo ""
echo "7. Stopping log aggregator..."
kill -SIGTERM $PID 2>/dev/null || true
wait $PID 2>/dev/null || true

echo ""
echo "8. Verifying checkpoint persistence..."
if [ -f /tmp/demo-checkpoints/positions.json ]; then
    echo "✓ Checkpoint persisted successfully"
else
    echo "✗ Checkpoint not persisted"
fi

echo ""
echo "=========================================="
echo "Demo completed!"
echo "=========================================="
echo ""
echo "Phase 1 Features Demonstrated:"
echo "  ✓ File tailing"
echo "  ✓ New line detection"
echo "  ✓ File rotation handling"
echo "  ✓ Checkpoint creation and persistence"
echo "  ✓ Structured logging"
echo ""
