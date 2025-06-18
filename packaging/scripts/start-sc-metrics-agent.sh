#!/bin/sh

# Function to log messages to systemd journal if systemd-cat is available
log_message() {
    if command -v systemd-cat >/dev/null 2>&1; then
        echo "$1" | systemd-cat -p info -t sc-metrics-agent-wrapper
    else
        echo "$1" >&2 # Fallback to stderr
    fi
}

log_message "Wrapper script started. Detecting VM ID..."

# Attempt to determine VM ID using dmidecode, with fallbacks
SC_VM_ID_DETECTED=$(/usr/sbin/dmidecode -s system-uuid 2>/dev/null)

if [ -z "$SC_VM_ID_DETECTED" ] || [ "$SC_VM_ID_DETECTED" = "" ]; then
    if [ -f /etc/machine-id ]; then
        SC_VM_ID_DETECTED=$(cat /etc/machine-id 2>/dev/null)
    fi
fi

if [ -z "$SC_VM_ID_DETECTED" ] || [ "$SC_VM_ID_DETECTED" = "" ]; then
    if [ -f /proc/sys/kernel/random/boot_id ]; then
        SC_VM_ID_DETECTED=$(cat /proc/sys/kernel/random/boot_id 2>/dev/null)
    fi
fi

if [ -z "$SC_VM_ID_DETECTED" ] || [ "$SC_VM_ID_DETECTED" = "" ]; then
    SC_VM_ID_DETECTED=$(hostname 2>/dev/null)
fi

if [ -z "$SC_VM_ID_DETECTED" ] || [ "$SC_VM_ID_DETECTED" = "" ]; then
    SC_VM_ID_DETECTED='unknown-vm-id'
fi

log_message "VM ID detected as: $SC_VM_ID_DETECTED"

export SC_VM_ID="$SC_VM_ID_DETECTED"
export SC_AGENT_CONFIG="/etc/sc-metrics-agent/config.yaml"
log_message "SC_VM_ID set to $SC_VM_ID"
log_message "SC_AGENT_CONFIG set to $SC_AGENT_CONFIG"

AGENT_BINARY="/usr/local/bin/sc-metrics-agent"

if [ ! -f "$AGENT_BINARY" ]; then
    log_message "ERROR: Agent binary $AGENT_BINARY not found!"
    exit 127 # Standard "command not found"
fi

if [ ! -x "$AGENT_BINARY" ]; then
    log_message "ERROR: Agent binary $AGENT_BINARY is not executable! Listing details:"
    ls -l "$AGENT_BINARY" | log_message # Log permissions for debugging
    exit 126 # Standard "command invoked cannot execute"
fi

log_message "Attempting to execute $AGENT_BINARY $*"
# Execute the main agent binary, passing all arguments
exec "$AGENT_BINARY" "$@"

# This part should ideally not be reached if exec succeeds or fails at kernel level.
# If exec itself (the shell built-in) fails (e.g. binary not found by shell after path search, which is not the case here as we use absolute path),
# the shell would exit with a specific code (like 127 or 126).
# If the kernel fails to load the binary (e.g. wrong arch, format error), systemd sees 203/EXEC.
log_message "FATAL: exec $AGENT_BINARY failed unexpectedly post-checks! This indicates a severe issue."
exit 1 # Generic error if exec somehow returns (highly unlikely for a successful exec)