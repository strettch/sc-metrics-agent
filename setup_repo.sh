#!/bin/bash
set -e

# --- Configuration ---
ORG_NAME="strettch"
GPG_EMAIL="engineering@strettch.com"
PACKAGE_NAME="sc-metrics-agent"
PACKAGE_VERSION="1.0.2" # Increment this for new versions
REPO_DOMAIN="repo.cloud.strettch.dev"
# ---

# --- Asset Paths (uses files from your repo) ---
SERVICE_FILE="packaging/systemd/${PACKAGE_NAME}.service"
POSTINSTALL_SCRIPT="packaging/scripts/post-install.sh"
PREREMOVE_SCRIPT="packaging/scripts/pre-remove.sh"
# ---

# --- Pre-flight Checks ---
if [ ! -f "$SERVICE_FILE" ]; then
    echo "Error: Service file not found at ${SERVICE_FILE}" >&2
    exit 1
fi
if [ ! -f "$POSTINSTALL_SCRIPT" ]; then
    echo "Error: Post-install script not found at ${POSTINSTALL_SCRIPT}" >&2
    exit 1
fi
if [ ! -f "$PREREMOVE_SCRIPT" ]; then
    echo "Error: Pre-remove script not found at ${PREREMOVE_SCRIPT}" >&2
    exit 1
fi
# ---

echo "--- [Step 1/6] Cleaning up previous build artifacts and repository..."
aptly publish drop focal || true
if aptly snapshot list -raw | grep -q .; then
    aptly snapshot list -raw | xargs --no-run-if-empty aptly snapshot delete
fi
aptly repo remove sc-metrics-agent-repo ${PACKAGE_NAME} || true
rm -f ${PACKAGE_NAME}_*.deb

echo "--- [Step 2/6] Building Go binary..."
GOOS=linux GOARCH=amd64 go build -o ${PACKAGE_NAME} ./cmd/agent/main.go

echo "--- [Step 3/6] Preparing packaging assets..."
STAGING_DIR="/tmp/${PACKAGE_NAME}-build"
rm -rf "${STAGING_DIR}"
mkdir -p "${STAGING_DIR}/usr/local/bin"
mkdir -p "${STAGING_DIR}/etc/${PACKAGE_NAME}"
mkdir -p "${STAGING_DIR}/etc/systemd/system"

cp ${PACKAGE_NAME} "${STAGING_DIR}/usr/local/bin/"
cp config.example.yaml "${STAGING_DIR}/etc/${PACKAGE_NAME}/config.yaml"
cp "${SERVICE_FILE}" "${STAGING_DIR}/etc/systemd/system/"
chmod +x "${POSTINSTALL_SCRIPT}" "${PREREMOVE_SCRIPT}"

echo "--- [Step 4/6] Building the .deb package with FPM..."
fpm -s dir -t deb -n ${PACKAGE_NAME} -v ${PACKAGE_VERSION} \
  -C "${STAGING_DIR}/" \
  --description "SC Metrics Agent for system monitoring by ${ORG_NAME}" \
  --maintainer "${GPG_EMAIL}" \
  --url "https://github.com/strettch/sc-metrics-agent" \
  --depends "libc6" --depends "dmidecode" \
  --after-install "${POSTINSTALL_SCRIPT}" \
  --before-remove "${PREREMOVE_SCRIPT}"

echo "--- [Step 5/6] Setting up and publishing the Aptly repository..."
if ! aptly repo show sc-metrics-agent-repo > /dev/null 2>&1; then
    aptly repo create -distribution="focal" -component="main" sc-metrics-agent-repo
fi
aptly repo add sc-metrics-agent-repo ${PACKAGE_NAME}_${PACKAGE_VERSION}_amd64.deb
SNAPSHOT_NAME="${PACKAGE_NAME}-${PACKAGE_VERSION}"
aptly snapshot create "${SNAPSHOT_NAME}" from repo sc-metrics-agent-repo
aptly publish snapshot -gpg-key="${GPG_EMAIL}" "${SNAPSHOT_NAME}"

GPG_PUBLIC_KEY_FILE="${PACKAGE_NAME}-repo.gpg"
gpg --armor --export "${GPG_EMAIL}" > "${GPG_PUBLIC_KEY_FILE}"
mv "${GPG_PUBLIC_KEY_FILE}" ~/.aptly/public/

echo "--- [Step 6/6] Generating Caddy config and client install.sh script..."
CADDY_USER_HOME=$(eval echo ~$(logname))
CADDY_ROOT_DIR="${CADDY_USER_HOME}/.aptly/public"
cat << EOF > /etc/caddy/Caddyfile
${REPO_DOMAIN} {
    root * ${CADDY_ROOT_DIR}
    file_server
}
EOF
systemctl restart caddy

INSTALL_SCRIPT_PATH="${CADDY_ROOT_DIR}/install.sh"
CONFIG_FILE_PATH="/etc/${PACKAGE_NAME}/config.yaml"
cat << EOF > "${INSTALL_SCRIPT_PATH}"
#!/bin/sh
set -e

# --- Configuration ---
REPO_HOST="${REPO_DOMAIN}"
PACKAGE_NAME="${PACKAGE_NAME}"
GPG_KEY_FILENAME="${GPG_PUBLIC_KEY_FILE}"
CONFIG_FILE="${CONFIG_FILE_PATH}"
# ---

if [ "\$(id -u)" -ne "0" ]; then
    echo "This script must be run as root. Please use 'sudo'." >&2
    exit 1
fi

echo "--- Installing \${PACKAGE_NAME} ---"
apt-get update
apt-get install -y apt-transport-https ca-certificates curl gnupg

KEYRING_FILE="/usr/share/keyrings/\${PACKAGE_NAME}-keyring.gpg"
curl -fsSL "https://\${REPO_HOST}/\${GPG_KEY_FILENAME}" | gpg --dearmor -o "\${KEYRING_FILE}"

SOURCES_FILE="/etc/apt/sources.list.d/\${PACKAGE_NAME}.list"
echo "deb [signed-by=\${KEYRING_FILE}] https://\${REPO_HOST} focal main" > "\${SOURCES_FILE}"

echo "Updating package list for \${PACKAGE_NAME}..."
apt-get update \
  -o Dir::Etc::SourceList="\${SOURCES_FILE}" \
  -o Dir::Etc::SourceParts="-" \
  -o APT::Get::List-Cleanup="0"

echo "Installing \${PACKAGE_NAME}..."
apt-get install -y "\${PACKAGE_NAME}"

echo
echo "---"
echo "✅ \${PACKAGE_NAME} was installed successfully!"
echo
echo "IMPORTANT: Please edit the configuration file to set your ingestor endpoint:"
echo "   sudo nano \${CONFIG_FILE}"
echo "After editing, restart the agent: sudo systemctl restart \${PACKAGE_NAME}"
echo "To check status: systemctl status \${PACKAGE_NAME}"
echo "---"

exit 0
EOF
chmod +x "${INSTALL_SCRIPT_PATH}"

echo
echo "========================================================================"
echo "✅ Repository setup complete!"
echo
echo "To install the agent on a client machine, run:"
echo "curl -sSL https://${REPO_DOMAIN}/install.sh | sudo bash"
echo "========================================================================"
