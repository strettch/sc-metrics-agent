#!/bin/bash
# scripts/deploy-to-repo.sh - Deploy to repository server
set -euo pipefail

VERSION="${1:?Version required}"
CHANNEL="${2:?Channel required}"
DEPLOY_KEY="${3:?Deploy key required}"
REPO_HOST="${4:?Repository host required}"
REPO_USER="${5:?Repository user required}"
# Optional branch to deploy (default: main)
BRANCH="${6:-main}"

# Setup SSH securely
setup_ssh() {
    mkdir -p ~/.ssh
    echo "$DEPLOY_KEY" > ~/.ssh/deploy_key
    chmod 600 ~/.ssh/deploy_key
    
    # Add host key verification
    ssh-keyscan -H "$REPO_HOST" >> ~/.ssh/known_hosts 2>/dev/null
    
    # Create SSH config for this connection
    cat > ~/.ssh/config <<-EOF
	Host repo-server
	    HostName $REPO_HOST
	    User $REPO_USER
	    IdentityFile ~/.ssh/deploy_key
	    StrictHostKeyChecking yes
	    ConnectTimeout 10
	    ServerAliveInterval 60
	EOF
}

# Deploy to repository
deploy() {
    echo "Deploying version $VERSION to $CHANNEL repository..."
    
	# Update repository code on remote using the requested branch
	ssh repo-server bash -s <<-REMOTE_SCRIPT
		set -euo pipefail

		cd /root/sc-metrics-agent || exit 1

		# Update git repository for the requested branch
		echo "Updating repository... (branch: $BRANCH)"
		git fetch origin "$BRANCH" --tags || true
		# Create or reset local branch from origin/<branch>
		if git show-ref --verify --quiet "refs/remotes/origin/$BRANCH"; then
			git checkout -B "$BRANCH" "origin/$BRANCH"
		else
			git checkout -B "$BRANCH"
		fi
		
		# Clean up old tags
		git tag -d v* 2>/dev/null || true
		
		# Run setup with proper environment
		echo "Building and publishing packages..."
		export RELEASE_TYPE="$CHANNEL"
		export PACKAGE_VERSION="$VERSION"
		export DEPLOY_BRANCH="$BRANCH"
		
		if ./setup_repo.sh; then
		    echo "✅ Deployment successful"
		else
		    echo "❌ Deployment failed"
		    exit 1
		fi
		
		# Validate deployment
		if [ "$CHANNEL" = "beta" ]; then
		    INSTALL_PATH="/srv/repo/public/metrics/beta/install.sh"
		else
		    INSTALL_PATH="/srv/repo/public/metrics/install.sh"
		fi
		
		if [ ! -f "\$INSTALL_PATH" ]; then
		    echo "❌ Install script not found at \$INSTALL_PATH"
		    exit 1
		fi
		
		echo "✅ All validations passed"
	REMOTE_SCRIPT
}

# Cleanup function
cleanup() {
    rm -f ~/.ssh/deploy_key
    rm -f ~/.ssh/config
}

# Main execution
trap cleanup EXIT
setup_ssh
deploy

echo "Deployment completed for $VERSION ($CHANNEL)"