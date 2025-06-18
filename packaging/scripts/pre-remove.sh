#!/bin/sh

set -e # Exit immediately if a command exits with a non-zero status.

if [ "$1" = "purge" ]; then
    echo "Purging sc-metrics-agent..."

    echo "Stopping and disabling sc-metrics-agent service..."
    systemctl stop sc-metrics-agent.service || true # Allow failure if already stopped
    systemctl disable sc-metrics-agent.service || true # Allow failure if already disabled

    echo "Removing configuration files..."
    rm -rf /etc/sc-metrics-agent

    echo "Removing systemd service file..."
    rm -f /etc/systemd/system/sc-metrics-agent.service

    echo "Reloading systemd daemon..."
    systemctl daemon-reload || true # Allow failure if systemd is not available (e.g. in a container)

    echo "sc-metrics-agent purge script finished."
else
    echo "Stopping and disabling sc-metrics-agent (on remove/upgrade)..."
    systemctl stop sc-metrics-agent.service || true
    systemctl disable sc-metrics-agent.service || true
fi

exit 0
