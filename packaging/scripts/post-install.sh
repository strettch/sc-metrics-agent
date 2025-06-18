#!/bin/sh
echo "Reloading systemd, enabling and starting sc-metrics-agent..."
systemctl daemon-reload
systemctl enable sc-metrics-agent.service
systemctl reset-failed sc-metrics-agent.service || true # Clear any failed state from previous attempts
systemctl start sc-metrics-agent.service || echo "sc-metrics-agent.service could not be started immediately, but installation will continue. Please check 'systemctl status sc-metrics-agent.service' and 'journalctl -u sc-metrics-agent.service' for details." # Allow start to fail
