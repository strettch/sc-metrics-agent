#!/bin/bash
# SC-Agent Auto-Update Script
# Purpose: Check for and install updates ONLY for sc-metrics-agent package
# Triggered by: sc-metrics-agent-update-scheduler.timer

set -euo pipefail

# Configuration
readonly PACKAGE_NAME="sc-metrics-agent"
readonly LOG_PREFIX="[SC-Metrics-Agent-Updater]"
readonly LOCK_FILE="/var/lock/sc-metrics-agent-updater.lock"
readonly CONFIG_FILE="/etc/${PACKAGE_NAME}/config.yaml"
readonly MAX_RETRIES=3
readonly RETRY_DELAY=30

# Ensure we're running as root
if [ "$EUID" -ne 0 ]; then 
    echo "${LOG_PREFIX} Error: Must run as root" >&2
    exit 1
fi

# Logging function
log() {
    local level=$1
    shift
    local message="$@"
    local timestamp=$(date -u +"%Y-%m-%d %H:%M:%S UTC")
    
    # Use appropriate output stream
    case $level in
        ERROR|WARN)
            echo "${LOG_PREFIX} ${timestamp} [${level}] ${message}" >&2
            ;;
        *)
            echo "${LOG_PREFIX} ${timestamp} [${level}] ${message}"
            ;;
    esac
}

# Cleanup function (preserving original atomic lock approach)
cleanup() {
    rmdir "${LOCK_FILE}" 2>/dev/null || true
}

# Set up exit trap
trap cleanup EXIT INT TERM

# Check if another instance is running (using atomic mkdir approach from original)
if ! mkdir "${LOCK_FILE}" 2>/dev/null; then
    log "WARN" "Another updater instance is running"
    exit 0
fi

# Start update process
log "INFO" "Starting update check for ${PACKAGE_NAME}"

# Check OS compatibility
if [ ! -f /etc/debian_version ]; then
    log "ERROR" "Not a Debian/Ubuntu system"
    exit 1
fi

# Add jitter to prevent thundering herd
JITTER=$((RANDOM % 60))
log "INFO" "Adding ${JITTER}s jitter delay"
sleep ${JITTER}

# Set APT environment
export DEBIAN_FRONTEND=noninteractive
export SC_AGENT_AUTO_UPDATER=1

# Function to check package health
check_package_health() {
    local status=$(dpkg -l ${PACKAGE_NAME} 2>/dev/null | grep "^[ih]" | awk '{print $1}')
    
    if [[ -z "$status" ]]; then
        log "ERROR" "Package not found in dpkg database"
        return 1
    elif [[ "$status" == "ii" ]]; then
        log "INFO" "Package health: OK (status: ii)"
        return 0
    elif [[ "$status" =~ [FRU] ]]; then
        log "WARN" "Package needs repair (status: ${status})"
        return 2
    else
        log "WARN" "Package status unknown: ${status}"
        return 2
    fi
}

# Function to repair package
repair_package() {
    log "INFO" "Attempting package repair..."
    
    # Try reconfiguration first
    if dpkg --configure ${PACKAGE_NAME} 2>&1; then
        log "INFO" "Package reconfiguration successful"
        return 0
    fi
    
    # Try reinstall
    log "INFO" "Reconfiguration failed, attempting reinstall..."
    if apt-get install --reinstall -y \
        -o DPkg::Lock::Timeout=300 \
        -o Dpkg::Options::="--force-confdef" \
        -o Dpkg::Options::="--force-confold" \
        ${PACKAGE_NAME} 2>&1; then
        log "INFO" "Package reinstall successful"
        return 0
    fi
    
    log "ERROR" "Package repair failed"
    return 1
}

# Check and repair package if needed
check_package_health
HEALTH_STATUS=$?

if [ ${HEALTH_STATUS} -eq 1 ]; then
    log "ERROR" "Package not installed, cannot update"
    exit 1
elif [ ${HEALTH_STATUS} -eq 2 ]; then
    repair_package || exit 1
fi

# Remember if service was running before update
SERVICE_WAS_RUNNING=false
if systemctl is-active --quiet "${PACKAGE_NAME}"; then
    SERVICE_WAS_RUNNING=true
    log "INFO" "Service is currently running, will restart after update"
else
    log "INFO" "Service is not running, will remain stopped after update"
fi

# Update package lists with retry
for attempt in $(seq 1 ${MAX_RETRIES}); do
    log "INFO" "Updating package lists (attempt ${attempt}/${MAX_RETRIES})"
    
    # Clean package cache for fresh data
    apt-get clean
    
    if apt-get update 2>&1; then
        log "INFO" "Package list update successful"
        break
    else
        if [ ${attempt} -lt ${MAX_RETRIES} ]; then
            log "WARN" "Package list update failed, retrying in ${RETRY_DELAY}s..."
            sleep ${RETRY_DELAY}
        else
            log "ERROR" "Package list update failed after ${MAX_RETRIES} attempts"
            exit 1
        fi
    fi
done

# Get version information
CURRENT_VERSION=$(dpkg-query -W -f='${Version}' ${PACKAGE_NAME} 2>/dev/null || echo "unknown")
log "INFO" "Current version: ${CURRENT_VERSION}"

# Check available version using apt-cache policy
AVAILABLE_VERSION=$(apt-cache policy ${PACKAGE_NAME} 2>/dev/null | grep "Candidate:" | awk '{print $2}' || echo "unknown")
log "INFO" "Available version: ${AVAILABLE_VERSION}"

# Verify we got valid version info
if [ "${AVAILABLE_VERSION}" = "unknown" ] || [ "${AVAILABLE_VERSION}" = "(none)" ]; then
    log "ERROR" "Could not determine available version"
    exit 1
fi

# Check if update is needed
if [ "${CURRENT_VERSION}" = "${AVAILABLE_VERSION}" ]; then
    log "INFO" "Already running latest version (${CURRENT_VERSION})"
    exit 0
fi

# Perform update
log "INFO" "Updating from ${CURRENT_VERSION} to ${AVAILABLE_VERSION}"

# Update with retry logic
UPDATE_SUCCESS=false
for attempt in $(seq 1 ${MAX_RETRIES}); do
    log "INFO" "Installation attempt ${attempt}/${MAX_RETRIES}"
    
    if apt-get install -y \
        -o DPkg::Lock::Timeout=300 \
        -o Dpkg::Options::="--force-confdef" \
        -o Dpkg::Options::="--force-confold" \
        -o APT::Get::AutomaticRemove=false \
        ${PACKAGE_NAME} 2>&1; then
        
        # Verify update
        NEW_VERSION=$(dpkg-query -W -f='${Version}' ${PACKAGE_NAME} 2>/dev/null || echo "unknown")
        
        if [ "${NEW_VERSION}" != "${CURRENT_VERSION}" ] && [ "${NEW_VERSION}" != "unknown" ]; then
            log "INFO" "Update successful: ${CURRENT_VERSION} -> ${NEW_VERSION}"
            UPDATE_SUCCESS=true
            break
        else
            log "ERROR" "Update verification failed (version unchanged)"
        fi
    else
        log "ERROR" "Installation attempt ${attempt} failed"
    fi
    
    if [ ${attempt} -lt ${MAX_RETRIES} ]; then
        log "INFO" "Retrying in ${RETRY_DELAY}s..."
        sleep ${RETRY_DELAY}
        
        # Try to fix any issues before retry
        dpkg --configure -a 2>&1 || true
        apt-get install -f -y 2>&1 || true
    fi
done

if [ "${UPDATE_SUCCESS}" = true ]; then
    if [ -f "${CONFIG_FILE}" ]; then
        log "INFO" "Validating configuration..."
        if command -v "/usr/bin/${PACKAGE_NAME}" >/dev/null 2>&1; then
            if ! "/usr/bin/${PACKAGE_NAME}" --validate-config "${CONFIG_FILE}" 2>/dev/null; then
                log "WARN" "Configuration validation failed, but continuing with update"
            else
                log "INFO" "Configuration validation successful"
            fi
        else
            log "INFO" "Binary not found, skipping config validation"
        fi
    fi

    # Check if service needs restart and restart if it was running before
    if [ "$SERVICE_WAS_RUNNING" = true ]; then
        if systemctl is-active --quiet ${PACKAGE_NAME} 2>/dev/null; then
            log "INFO" "Restarting service to apply update..."
            
            # Give service time to shut down gracefully
            if systemctl restart ${PACKAGE_NAME} 2>&1; then
                sleep 2
                
                # Verify service is running
                if systemctl is-active --quiet ${PACKAGE_NAME} 2>/dev/null; then
                    log "INFO" "Service restarted successfully"
                else
                    log "ERROR" "Service failed to restart after update"
                    log "INFO" "Attempting to start service..."
                    systemctl start ${PACKAGE_NAME} 2>&1 || true
                fi
            else
                log "ERROR" "Failed to restart service"
            fi
        else
            log "INFO" "Service was running but not active after update, attempting to start..."
            systemctl start ${PACKAGE_NAME} 2>&1 || true
        fi
    else
        log "INFO" "Service was not running before update, leaving stopped"
    fi
    
    log "INFO" "Update completed successfully"
    exit 0
else
    log "ERROR" "Update failed after ${MAX_RETRIES} attempts"
    exit 1
fi