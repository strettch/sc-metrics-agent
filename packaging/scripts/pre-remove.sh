#!/bin/sh
echo "Stopping and disabling sc-metrics-agent..."
systemctl stop sc-metrics-agent.service
systemctl disable sc-metrics-agent.service
