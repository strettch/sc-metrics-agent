#!/bin/bash
# Repository Setup Script
# Purpose: Build packages and set up APT repository on the repository server
# Usage: ./setup_repo.sh (run on repository server)
# Location: This script runs on the repository server at /root/sc-metrics-agent/

set -e

# --- Configuration ---
ORG_NAME="strettch"
GPG_EMAIL="engineering@strettch.com"
PACKAGE_NAME="sc-metrics-agent"
KEEP_VERSIONS=5  # Number of versions to keep in repository

# Use override version if provided, otherwise get from git
if [ -n "${OVERRIDE_VERSION:-}" ]; then
    PACKAGE_VERSION="$OVERRIDE_VERSION"
    echo "Using override version: $PACKAGE_VERSION"
else
    # Get the latest tag to use as version
    LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
    if [ -n "$LATEST_TAG" ]; then
        PACKAGE_VERSION="${LATEST_TAG#v}"  # Remove 'v' prefix
        echo "Using version from latest tag: $PACKAGE_VERSION"
    else
        # Fallback to git describe if no tags
        PACKAGE_VERSION=$(git describe --tags --always --dirty)
        echo "No tags found, using git describe: $PACKAGE_VERSION"
    fi
fi

# Clean up old packages, but keep the one we're about to process
if [ -n "${OVERRIDE_VERSION:-}" ]; then
    # Remove old packages but keep the current one
    find . -name "${PACKAGE_NAME}_*.deb" ! -name "${PACKAGE_NAME}_${PACKAGE_VERSION}_amd64.deb" -delete 2>/dev/null || true
    echo "Keeping package: ${PACKAGE_NAME}_${PACKAGE_VERSION}_amd64.deb"
else
    # Traditional cleanup when building locally
    rm -f ${PACKAGE_NAME}_*.deb
fi

REPO_DOMAIN="repo.cloud.strettch.dev"  # Production repository domain
DISTRIBUTIONS="bionic focal jammy noble oracular"  # Ubuntu versions to support
WEB_ROOT_DIR="/srv/repo/public/metrics"
# ---

# --- Asset Paths ---
SERVICE_FILE="packaging/systemd/${PACKAGE_NAME}.service"
UPDATER_SERVICE="packaging/systemd/${PACKAGE_NAME}-update-scheduler.service"
UPDATER_TIMER="packaging/systemd/${PACKAGE_NAME}-update-scheduler.timer"
UPDATER_SCRIPT="packaging/scripts/${PACKAGE_NAME}-updater.sh"
POSTINSTALL_SCRIPT="packaging/scripts/post-install.sh"
PREREMOVE_SCRIPT="packaging/scripts/pre-remove.sh"
START_SCRIPT_FILENAME="start-sc-metrics-agent.sh"
START_SCRIPT_SOURCE_PATH="packaging/scripts/${START_SCRIPT_FILENAME}"
# ---

# Check for required tools first
for tool in fpm aptly gpg; do
    if ! command -v $tool &> /dev/null; then
        echo "Error: $tool is not installed" >&2
        exit 1
    fi
done

echo "--- [Step 1/7] Cleaning up previous repository publications..."
# Note: Package cleanup was already handled above based on OVERRIDE_VERSION

if [ -n "${OVERRIDE_VERSION:-}" ]; then
    echo "--- [Step 2/7] Using uploaded package (skipping build)..."
    EXPECTED_PACKAGE="${PACKAGE_NAME}_${PACKAGE_VERSION}_amd64.deb"
    if [ ! -f "$EXPECTED_PACKAGE" ]; then
        echo "Error: Expected package file not found: $EXPECTED_PACKAGE"
        exit 1
    fi
    echo "Found uploaded package: $EXPECTED_PACKAGE"
else
    echo "--- [Step 2/7] Building Go binary via Makefile..."
    # Clean up packages when building locally
    rm -f ${PACKAGE_NAME}_*.deb
    # Force VERSION to use the clean tag version
    GOOS=linux GOARCH=amd64 make build VERSION=v${PACKAGE_VERSION}
fi

if [ -z "${OVERRIDE_VERSION:-}" ]; then
    # --- Pre-flight Checks ---
    echo "Checking required files..."
    required_files=(
        "$SERVICE_FILE"
        "$UPDATER_SERVICE"
        "$UPDATER_TIMER"
        "$UPDATER_SCRIPT"
        "$POSTINSTALL_SCRIPT"
        "$PREREMOVE_SCRIPT"
        "$START_SCRIPT_SOURCE_PATH"
        "build/${PACKAGE_NAME}"
    )

    for file in "${required_files[@]}"; do
        if [ ! -f "$file" ]; then
            echo "Error: Required file not found: ${file}" >&2
            echo "Run 'make build' first if binary is missing" >&2
            exit 1
        fi
    done
    # ---

    echo "--- [Step 3/7] Preparing packaging assets..."
STAGING_DIR="/tmp/${PACKAGE_NAME}-build"
rm -rf "${STAGING_DIR}"
mkdir -p "${STAGING_DIR}/usr/bin" "${STAGING_DIR}/usr/local/bin" "${STAGING_DIR}/etc/systemd/system"

# Copy files
cp "build/${PACKAGE_NAME}" "${STAGING_DIR}/usr/local/bin/"
chmod +x "${STAGING_DIR}/usr/local/bin/${PACKAGE_NAME}"

# Add start script
cp "${START_SCRIPT_SOURCE_PATH}" "${STAGING_DIR}/usr/local/bin/${START_SCRIPT_FILENAME}"
chmod +x "${STAGING_DIR}/usr/local/bin/${START_SCRIPT_FILENAME}"

# Add updater script
cp "${UPDATER_SCRIPT}" "${STAGING_DIR}/usr/bin/"
chmod +x "${STAGING_DIR}/usr/bin/${PACKAGE_NAME}-updater.sh"

# Add systemd files
cp "${SERVICE_FILE}" "${STAGING_DIR}/etc/systemd/system/"
cp "${UPDATER_SERVICE}" "${STAGING_DIR}/etc/systemd/system/"
cp "${UPDATER_TIMER}" "${STAGING_DIR}/etc/systemd/system/"

# Build .deb package with FPM
fpm -s dir -t deb \
    -n ${PACKAGE_NAME} \
    -v ${PACKAGE_VERSION} \
    -C "${STAGING_DIR}/" \
    --description "SC Metrics Agent for system monitoring by ${ORG_NAME}" \
    --maintainer "${GPG_EMAIL}" \
    --url "https://github.com/strettch/sc-metrics-agent" \
    --depends "libc6" --depends "dmidecode" \
    --after-install "${POSTINSTALL_SCRIPT}" \
    --before-remove "${PREREMOVE_SCRIPT}"
fi

echo "--- [Step 4/7] Adding package to APT repository..."
if ! sudo aptly repo show sc-metrics-agent-repo > /dev/null 2>&1; then
    sudo aptly repo create -distribution="focal" -component="main" sc-metrics-agent-repo
fi

# Clean up old versions before adding new one
echo "Checking for old versions to remove (keeping last ${KEEP_VERSIONS} versions)..."
CURRENT_VERSIONS=$(sudo aptly repo show -with-packages sc-metrics-agent-repo 2>/dev/null | grep "^  ${PACKAGE_NAME}_" | sed 's/^  //' | sort -V)
VERSION_COUNT=$(echo "$CURRENT_VERSIONS" | grep -c . || echo 0)

if [ $VERSION_COUNT -ge $KEEP_VERSIONS ]; then
    VERSIONS_TO_REMOVE=$((VERSION_COUNT - KEEP_VERSIONS + 1))
    echo "Found $VERSION_COUNT versions, removing $VERSIONS_TO_REMOVE oldest versions..."
    
    # Get the versions to remove (oldest ones)
    OLD_VERSIONS=$(echo "$CURRENT_VERSIONS" | head -n $VERSIONS_TO_REMOVE)
    
    for old_ver in $OLD_VERSIONS; do
        echo "Removing old version: $old_ver"
        sudo aptly repo remove sc-metrics-agent-repo "$old_ver" || true
    done
fi

sudo aptly repo add sc-metrics-agent-repo ${PACKAGE_NAME}_${PACKAGE_VERSION#v}_amd64.deb
SNAPSHOT_NAME="${PACKAGE_NAME}-${PACKAGE_VERSION}"
sudo aptly snapshot create "${SNAPSHOT_NAME}" from repo sc-metrics-agent-repo

# --- Configure GPG environment ---
export GPG_TTY=$(tty)
unset DISPLAY

# Get the fingerprint for the GPG key
GPG_FINGERPRINT=$(gpg --list-secret-keys --with-colons "${GPG_EMAIL}" | awk -F: '/^sec/{print $5; exit}')
if [ -z "${GPG_FINGERPRINT}" ]; then
    echo "Error: Could not find GPG key for ${GPG_EMAIL}" >&2
    exit 1
fi

# Publish snapshots with correct fingerprint
for dist in ${DISTRIBUTIONS}; do
    echo "Publishing for ${dist}..."
    # Drop any existing publication first to avoid conflicts
    sudo aptly publish drop "${dist}" 2>/dev/null || true
    sudo aptly publish snapshot -gpg-key="${GPG_FINGERPRINT}" -distribution="${dist}" -batch "${SNAPSHOT_NAME}"
done

echo "--- [Step 5/7] Creating and Publishing the new version..."
sudo mkdir -p "${WEB_ROOT_DIR}/aptly"
sudo rsync -a --delete ~/.aptly/public/ "${WEB_ROOT_DIR}/aptly/"

# Export GPG public key
GPG_PUBLIC_KEY_FILE="${PACKAGE_NAME}-repo.gpg"
gpg --armor --export "${GPG_EMAIL}" > "${GPG_PUBLIC_KEY_FILE}"
sudo mv "${GPG_PUBLIC_KEY_FILE}" "${WEB_ROOT_DIR}/"

# Generate dynamic install.sh
INSTALL_SCRIPT_PATH="${WEB_ROOT_DIR}/install.sh"
TMP_INSTALL_SCRIPT=$(mktemp)
cat << 'INSTALL_EOF' > "${TMP_INSTALL_SCRIPT}"
#!/bin/sh
set -e
REPO_HOST="repo.cloud.strettch.dev"
REPO_PATH="metrics"
PACKAGE_NAME="sc-metrics-agent"
GPG_KEY_FILENAME="sc-metrics-agent-repo.gpg"
CONFIG_FILE="/etc/sc-metrics-agent/config.yaml"

if [ "$(id -u)" -ne "0" ]; then
    echo "Run as root" >&2
    exit 1
fi

if [ -f /etc/os-release ]; then
    DISTRIBUTION=$(. /etc/os-release && echo "$VERSION_CODENAME")
else
    echo "Error: Cannot detect OS codename" >&2
    exit 1
fi

# Fallback logic for unsupported distributions
SUPPORTED_DISTS="bionic focal jammy noble oracular"
if ! echo "$SUPPORTED_DISTS" | grep -q "$DISTRIBUTION"; then
    echo "Warning: $DISTRIBUTION is not explicitly supported, falling back to noble (Ubuntu 24.04)"
    DISTRIBUTION="noble"
fi

echo "--- Installing ${PACKAGE_NAME} for ${DISTRIBUTION} ---"

log_and_run() {
    echo "$1"
    if ! eval "$2"; then
        echo "Error: Failed to $3" >&2
        exit 1
    fi
}

# Clean up any old sc-agent configurations
echo "Cleaning up old configurations..."
rm -f /etc/apt/sources.list.d/sc-agent.list
rm -f /usr/share/keyrings/sc-agent-keyring.gpg
rm -f /etc/apt/sources.list.d/sc-metrics-agent.list 2>/dev/null || true
rm -f /usr/share/keyrings/sc-metrics-agent-keyring.gpg 2>/dev/null || true

log_and_run "Updating package lists..." "apt-get update > /dev/null" "update package lists"
log_and_run "Installing prerequisites..." "apt-get install -y apt-transport-https ca-certificates curl gnupg > /dev/null" "install prerequisites"

KEYRING_FILE="/usr/share/keyrings/${PACKAGE_NAME}-keyring.gpg"
log_and_run "Downloading GPG key..." "curl -fsSL \"https://${REPO_HOST}/${REPO_PATH}/${GPG_KEY_FILENAME}\" | gpg --dearmor | tee \"${KEYRING_FILE}\" >/dev/null" "download GPG key"

SOURCES_FILE="/etc/apt/sources.list.d/${PACKAGE_NAME}.list"
log_and_run "Adding repository..." "echo \"deb [signed-by=${KEYRING_FILE}] https://${REPO_HOST}/${REPO_PATH}/aptly ${DISTRIBUTION} main\" | tee \"${SOURCES_FILE}\" >/dev/null" "add repository"

log_and_run "Updating package index..." "apt-get update -o Dir::Etc::SourceList='${SOURCES_FILE}' -o Dir::Etc::SourceParts='-' -o APT::Get::List-Cleanup='0' > /dev/null" "update package index"

log_and_run "Installing ${PACKAGE_NAME}..." "apt-get install -y \"${PACKAGE_NAME}\"" "install ${PACKAGE_NAME}"

echo "✅ ${PACKAGE_NAME} installed successfully!"
exit 0
INSTALL_EOF

sudo mv "${TMP_INSTALL_SCRIPT}" "${INSTALL_SCRIPT_PATH}"
sudo chmod +x "${INSTALL_SCRIPT_PATH}"

sudo chown -R caddy:caddy /srv/repo/public 2>/dev/null || sudo chown -R www-data:www-data /srv/repo/public 2>/dev/null || true
sudo chmod -R 755 /srv/repo/public

echo "--- [Step 6/7] Repository files configured..."
echo "--- [Step 7/7] Finalizing setup..."
echo
echo "========================================================================"
echo "✅ Repository setup complete!"
echo "To install the agent on a client machine, run:"
echo "curl -sSL https://${REPO_DOMAIN}/metrics/install.sh | sudo bash"
echo "========================================================================"
