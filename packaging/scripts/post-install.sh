#!/bin/sh

SERVICE_NAME="sc-metrics-agent.service"

echo "Configuring ${SERVICE_NAME}..."

systemctl daemon-reload

if ! systemctl is-enabled --quiet "${SERVICE_NAME}"; then
    echo "Enabling ${SERVICE_NAME}..."
    systemctl enable "${SERVICE_NAME}"
else
    echo "${SERVICE_NAME} is already enabled."
fi

systemctl reset-failed "${SERVICE_NAME}" || true # Clear any failed state

if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    echo "Starting ${SERVICE_NAME}..."
    systemctl start "${SERVICE_NAME}" || echo "Warning: ${SERVICE_NAME} could not be started immediately. Installation will continue. Please check 'systemctl status ${SERVICE_NAME}' and 'journalctl -u ${SERVICE_NAME}' for details."
else
    echo "${SERVICE_NAME} is already active."
fi
