#!/bin/bash
# Post-install script for SC Metrics Agent
# Uses atomic operations to ensure reliable updates

set -e

PACKAGE_NAME="sc-metrics-agent"
SERVICE_NAME="${PACKAGE_NAME}.service"
CONFIG_FILE="/etc/${PACKAGE_NAME}/config.yaml"

echo "Configuring ${SERVICE_NAME}..."

# Validate configuration before proceeding
echo "Validating configuration..."
if ! "/usr/local/bin/${PACKAGE_NAME}" --validate-config "${CONFIG_FILE}"; then
    echo "ERROR: Configuration validation failed"
    echo "Post-install aborted - configuration is invalid"
    exit 1
fi

# Reload systemd daemon
systemctl daemon-reload

# Enable service if not already enabled
if ! systemctl is-enabled --quiet "${SERVICE_NAME}"; then
    echo "Enabling ${SERVICE_NAME}..."
    systemctl enable "${SERVICE_NAME}"
else
    echo "${SERVICE_NAME} is already enabled."
fi

# Clear any failed state
systemctl reset-failed "${SERVICE_NAME}" || true

# Start service if not already active
if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    echo "Starting ${SERVICE_NAME}..."
    systemctl start "${SERVICE_NAME}"
    
    # Wait a moment and verify it started successfully
    sleep 2
    if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
        echo "ERROR: ${SERVICE_NAME} failed to start"
        echo "Check status with: systemctl status ${SERVICE_NAME}"
        echo "Check logs with: journalctl -u ${SERVICE_NAME}"
        exit 1
    fi
    echo "${SERVICE_NAME} started successfully."
else
    echo "${SERVICE_NAME} is already active."
    echo "Restarting ${SERVICE_NAME} to apply any configuration changes..."
    systemctl restart "${SERVICE_NAME}"
    
    # Wait a moment and verify it restarted successfully
    sleep 2
    if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
        echo "ERROR: ${SERVICE_NAME} failed to restart"
        echo "Check status with: systemctl status ${SERVICE_NAME}"
        echo "Check logs with: journalctl -u ${SERVICE_NAME}"
        exit 1
    fi
    echo "${SERVICE_NAME} restarted successfully."
fi

echo "✅ ${PACKAGE_NAME} installation completed successfully"
