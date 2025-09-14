#!/bin/bash
set -e

# Default version increment type
INCREMENT_TYPE="patch"

# Parse command line arguments
if [ "$1" == "major" ] || [ "$1" == "minor" ] || [ "$1" == "patch" ]; then
  INCREMENT_TYPE=$1
elif [ -n "$1" ]; then
  echo "Usage: $0 [major|minor|patch]"
  echo "Defaults to 'patch' if no argument is provided."
  exit 1
fi

# Ensure the current branch is up-to-date and fetch all tags robustly
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
echo "Pulling latest changes for branch $CURRENT_BRANCH..."
git pull origin "$CURRENT_BRANCH" --ff-only || echo "Failed to pull or branch is up-to-date/diverged (ff-only). Continuing..."

echo "Fetching all tags from remote (pruning old, forcing update)..."
git fetch origin --prune --tags --force

# Get the latest tag by listing all semantic version tags, sorting them, and taking the last one
LATEST_TAG=$(git tag -l '[0-9]*.[0-9]*.[0-9]*' | sort -V | tail -n 1 2>/dev/null || echo "0.0.0")

# Use version from latest tag
CURRENT_VERSION=${LATEST_TAG}

# Split version into major, minor, patch
IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"

# Increment version based on type
case "$INCREMENT_TYPE" in
  major)
    MAJOR=$((MAJOR + 1))
    MINOR=0
    PATCH=0
    ;;
  minor)
    MINOR=$((MINOR + 1))
    PATCH=0
    ;;
  patch)
    PATCH=$((PATCH + 1))
    ;;
esac

NEW_VERSION="${MAJOR}.${MINOR}.${PATCH}"

echo "Current version: ${LATEST_TAG}"
echo "Incrementing: ${INCREMENT_TYPE}"
echo "New version: ${NEW_VERSION}"

# Confirm before tagging
read -p "Create and push tag ${NEW_VERSION}? (y/N): " CONFIRMATION
if [[ "$CONFIRMATION" != [yY] && "$CONFIRMATION" != [yY][eE][sS] ]]; then
  echo "Tagging aborted by user."
  exit 0
fi

# Create and push the new tag
GIT_COMMIT=$(git rev-parse HEAD)
echo "Attempting to delete local tag $NEW_VERSION (if it exists)..."
git tag -d "$NEW_VERSION" >/dev/null 2>&1 || true

echo "Attempting to delete remote tag $NEW_VERSION (if it exists)..."
git push origin --delete "$NEW_VERSION" >/dev/null 2>&1 || true

# Fetch again after delete attempts to ensure local ref state is current
echo "Re-fetching tags from remote to synchronize after potential deletions..."
git fetch origin --prune --tags --force

GIT_COMMIT_HASH=$(git rev-parse HEAD)
echo "Force tagging commit $GIT_COMMIT_HASH as $NEW_VERSION"
git tag -f -a "${NEW_VERSION}" -m "Release ${NEW_VERSION}" "$GIT_COMMIT_HASH"

echo "Force pushing tag $NEW_VERSION (refs/tags/${NEW_VERSION}) to origin..."
git push origin --force "refs/tags/${NEW_VERSION}"


echo "✅ Successfully created and pushed tag ${NEW_VERSION}."