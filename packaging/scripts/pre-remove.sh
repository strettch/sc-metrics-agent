#!/bin/bash
# SC-Agent Pre-Removal Script
# Purpose: Clean up services and files before package removal
# Triggered by: APT package manager before removing/upgrading sc-metrics-agent

set -euo pipefail

# Define service names
readonly SERVICE_NAME="sc-metrics-agent.service"
readonly UPDATER_SERVICE="sc-metrics-agent-update-scheduler.service" 
readonly UPDATER_TIMER="sc-metrics-agent-update-scheduler.timer"
readonly CONFIG_DIR="/etc/sc-metrics-agent"
readonly RUNTIME_DIR="/var/run/sc-metrics-agent"

# Get removal action from dpkg
ACTION="${1:-}"

# Helper function to stop service safely
stop_service() {
    local service=$1
    if systemctl is-active --quiet "$service" 2>/dev/null; then
        echo "Stopping $service..."
        systemctl stop "$service" 2>/dev/null || true # Allow failure if already stopped
    fi
}

# Helper function to disable service safely
disable_service() {
    local service=$1
    if systemctl is-enabled --quiet "$service" 2>/dev/null; then
        echo "Disabling $service..."
        systemctl disable "$service" 2>/dev/null || true # Allow failure if already disabled
    fi
}

# Main logic based on action
case "$ACTION" in
    purge|0)
        echo "Purging sc-metrics-agent..."
        
        # Stop all services
        stop_service "${UPDATER_TIMER}"
        stop_service "${UPDATER_SERVICE}"
        stop_service "${SERVICE_NAME}"
        
        # Disable all services
        disable_service "${UPDATER_TIMER}"
        disable_service "${UPDATER_SERVICE}"
        disable_service "${SERVICE_NAME}"
        
        # Remove all files - no backup, user wants complete removal
        echo "Removing configuration files..."
        rm -rf "$CONFIG_DIR"
        rm -rf "$RUNTIME_DIR"
        
        # Remove systemd service files explicitly (preserving original behavior)
        echo "Removing systemd service files..."
        rm -f /etc/systemd/system/sc-metrics-agent.service
        rm -f /etc/systemd/system/sc-metrics-agent-update-scheduler.service
        rm -f /etc/systemd/system/sc-metrics-agent-update-scheduler.timer
        
        # Clean up any remaining systemd files
        find /etc/systemd/system -name "sc-metrics-agent*" -delete 2>/dev/null || true
        
        # Reload systemd daemon
        echo "Reloading systemd daemon..."
        systemctl daemon-reload || true # Allow failure if systemd is not available (e.g. in a container)
        
        # Reset failed state for all services
        echo "Resetting failed state for sc-metrics-agent services..."
        systemctl reset-failed "${SERVICE_NAME}" || true # Clear any failed state
        systemctl reset-failed "${UPDATER_SERVICE}" || true
        systemctl reset-failed "${UPDATER_TIMER}" || true
        
        echo "sc-metrics-agent purge script finished."
        ;;
        
    remove)
        echo "Removing sc-metrics-agent (keeping configuration)..."
        
        # Stop and disable services
        stop_service "${UPDATER_TIMER}"
        stop_service "${UPDATER_SERVICE}"
        stop_service "${SERVICE_NAME}"
        
        disable_service "${UPDATER_TIMER}"
        disable_service "${UPDATER_SERVICE}"
        disable_service "${SERVICE_NAME}"
        
        # Only remove runtime files, keep config (preserving original behavior)
        rm -rf "$RUNTIME_DIR"
        
        echo "sc-metrics-agent removed (configuration preserved in $CONFIG_DIR)"
        ;;
        
    upgrade|1)
        echo "Preparing for sc-metrics-agent upgrade..."
        
        # Check if auto-updater is running this
        if [ "${SC_AGENT_AUTO_UPDATER:-0}" = "1" ]; then
            echo "Auto-updater detected, minimal service disruption..."
            stop_service "${SERVICE_NAME}"
        else
            echo "Manual upgrade, stopping all services..."
            stop_service "${UPDATER_TIMER}"
            stop_service "${UPDATER_SERVICE}"
            stop_service "${SERVICE_NAME}"
        fi
        
        # Services remain enabled for restart after upgrade
        ;;
        
    *)
        echo "Stopping and disabling sc-metrics-agent (on remove/upgrade)..."
        stop_service "${UPDATER_TIMER}"
        stop_service "${UPDATER_SERVICE}"
        stop_service "${SERVICE_NAME}"
        disable_service "${UPDATER_TIMER}"
        disable_service "${UPDATER_SERVICE}"
        disable_service "${SERVICE_NAME}"
        ;;
esac

exit 0
