#!/bin/bash

set -e

REPO_TYPE="$1"  # "beta" or "stable" (not used since we use existing setup_repo.sh)
PACKAGE_FILE="$2"

if [[ ! -f "$PACKAGE_FILE" ]]; then
    echo "Error: Package file $PACKAGE_FILE not found"
    exit 1
fi

# Validate required environment variables
required_vars=("REPO_SSH_KEY" "REPO_HOST" "REPO_USER")
for var in "${required_vars[@]}"; do
    if [[ -z "${!var}" ]]; then
        echo "Error: Environment variable $var is required"
        exit 1
    fi
done

echo "Deploying $PACKAGE_FILE using existing setup_repo.sh..."

# Setup SSH key
echo "Setting up SSH key..."
mkdir -p ~/.ssh

# Check if SSH key is provided
if [ -z "$REPO_SSH_KEY" ]; then
    echo "Error: REPO_SSH_KEY environment variable is empty"
    exit 1
fi

# Write the private key directly (assuming it's already in proper format)
echo "Writing SSH private key..."
echo "$REPO_SSH_KEY" > ~/.ssh/deploy_key

# Verify the key was written successfully
if [ ! -s ~/.ssh/deploy_key ]; then
    echo "Error: SSH key file is empty after writing"
    exit 1
fi

# Verify it looks like a valid SSH private key
if ! grep -q "BEGIN.*PRIVATE KEY" ~/.ssh/deploy_key; then
    echo "Error: SSH key doesn't appear to be a valid private key format"
    echo "Expected to find 'BEGIN.*PRIVATE KEY' header"
    exit 1
fi

chmod 600 ~/.ssh/deploy_key
echo "SSH key setup completed successfully"

ssh-keyscan -H "$REPO_HOST" >> ~/.ssh/known_hosts

# Copy package to server (replaces the existing one)
echo "Uploading package to repository server..."
scp -i ~/.ssh/deploy_key "$PACKAGE_FILE" "${REPO_USER}@${REPO_HOST}:/root/sc-metrics-agent/"

# Update repository and switch to appropriate branch
echo "Updating repository on server and switching to appropriate branch..."
if [ "$REPO_TYPE" = "beta" ]; then
    BRANCH="dev"
else
    BRANCH="main"
fi

ssh -i ~/.ssh/deploy_key "${REPO_USER}@${REPO_HOST}" "
    cd /root/sc-metrics-agent && 
    echo 'Cleaning up working directory...' &&
    git checkout . && 
    git clean -fd && 
    git fetch origin && 
    git checkout $BRANCH && 
    git pull origin $BRANCH &&
    echo 'Repository updated to branch: $BRANCH'
"

# Run the existing setup_repo.sh script on the server
echo "Running setup_repo.sh on repository server..."
ssh -i ~/.ssh/deploy_key "${REPO_USER}@${REPO_HOST}" "cd /root/sc-metrics-agent && ./setup_repo.sh"

# Cleanup
rm -f ~/.ssh/deploy_key

echo "✅ Deployment completed successfully using setup_repo.sh!"
echo "📦 Repository updated at: https://repo.cloud.strettch.dev/metrics/"
echo "🚀 Install command: curl -sSL https://repo.cloud.strettch.dev/metrics/install.sh | sudo bash"