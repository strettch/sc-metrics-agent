#!/usr/bin/env bash
# SC-Metrics-Agent Configuration Downloader
# Purpose: Download latest agent.yaml from repository server
# Triggered by: post-install.sh and sc-metrics-agent-updater.sh

set -euo pipefail

# Configuration
readonly CONFIG_DIR="/etc/strettchcloud/config"
readonly CONFIG_FILE="${CONFIG_DIR}/agent.yaml"
readonly REPO_URL="${SC_CONFIG_REPO_URL:-https://repo.cloud.strettch.com/agent/config}"
readonly LOG_PREFIX="[SC-Metrics-Agent-Config]"
readonly MAX_RETRIES=3
readonly RETRY_DELAY=5
readonly CURL_TIMEOUT=30

# Logging function
log() {
    local level=$1
    shift
    local message="$@"
    local timestamp=$(date -u +"%Y-%m-%d %H:%M:%S UTC")

    case $level in
        ERROR|WARN)
            echo "${LOG_PREFIX} ${timestamp} [${level}] ${message}" >&2
            ;;
        *)
            echo "${LOG_PREFIX} ${timestamp} [${level}] ${message}"
            ;;
    esac
}

# Detect environment from existing config or environment variable
detect_environment() {
    local env="${SC_ENVIRONMENT:-}"

    # If not set via env var, try to read from existing config
    if [ -z "$env" ] && [ -f "${CONFIG_FILE}" ]; then
        env=$(grep -E "^[[:space:]]*environment:" "${CONFIG_FILE}" | awk '{print $2}' | tr -d '"' || echo "")
    fi

    # Default to production if still not set
    if [ -z "$env" ]; then
        env="production"
        log "INFO" "No environment specified, defaulting to: ${env}"
    else
        log "INFO" "Detected environment: ${env}"
    fi

    echo "$env"
}

# Download configuration file
download_config() {
    local environment=$(detect_environment)
    local config_url="${REPO_URL}/${environment}/agent.yaml"
    local temp_file="${CONFIG_FILE}.tmp"

    log "INFO" "Downloading configuration from: ${config_url}"

    # Create config directory if it doesn't exist
    if [ ! -d "${CONFIG_DIR}" ]; then
        log "INFO" "Creating config directory: ${CONFIG_DIR}"
        mkdir -p "${CONFIG_DIR}"
        chmod 755 "${CONFIG_DIR}"
    fi

    # Download with retries
    for attempt in $(seq 1 ${MAX_RETRIES}); do
        log "INFO" "Download attempt ${attempt}/${MAX_RETRIES}"

        if curl -f -s -S -L \
            --max-time ${CURL_TIMEOUT} \
            --connect-timeout 10 \
            -o "${temp_file}" \
            "${config_url}"; then

            # Verify downloaded file is not empty
            if [ ! -s "${temp_file}" ]; then
                log "ERROR" "Downloaded file is empty"
                rm -f "${temp_file}"

                if [ ${attempt} -lt ${MAX_RETRIES} ]; then
                    log "INFO" "Retrying in ${RETRY_DELAY}s..."
                    sleep ${RETRY_DELAY}
                    continue
                else
                    return 1
                fi
            fi

            # Move temp file to final location
            mv "${temp_file}" "${CONFIG_FILE}"
            chmod 644 "${CONFIG_FILE}"

            log "INFO" "Configuration downloaded successfully"
            return 0
        else
            log "WARN" "Download failed (attempt ${attempt}/${MAX_RETRIES})"
            rm -f "${temp_file}"

            if [ ${attempt} -lt ${MAX_RETRIES} ]; then
                log "INFO" "Retrying in ${RETRY_DELAY}s..."
                sleep ${RETRY_DELAY}
            fi
        fi
    done

    log "ERROR" "Failed to download configuration after ${MAX_RETRIES} attempts"
    return 1
}

# Main execution
main() {
    log "INFO" "Starting configuration download"

    if download_config; then
        log "INFO" "Configuration download completed successfully"
        exit 0
    else
        log "ERROR" "Configuration download failed"
        exit 1
    fi
}

main "$@"
