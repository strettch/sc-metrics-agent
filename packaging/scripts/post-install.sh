#!/bin/sh
echo "Reloading systemd, enabling and starting sc-metrics-agent..."
systemctl daemon-reload
systemctl enable sc-metrics-agent.service
systemctl start sc-metrics-agent.service
