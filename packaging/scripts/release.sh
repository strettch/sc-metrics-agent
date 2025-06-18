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

# Get the latest tag
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")

# Remove 'v' prefix if it exists
CURRENT_VERSION=${LATEST_TAG#v}

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

NEW_VERSION="v${MAJOR}.${MINOR}.${PATCH}"

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
echo "Tagging commit ${GIT_COMMIT} as ${NEW_VERSION}"
git tag -a "${NEW_VERSION}" -m "Release ${NEW_VERSION}"

echo "Pushing tag ${NEW_VERSION} to remote..."
git push origin "${NEW_VERSION}"

echo "✅ Successfully created and pushed tag ${NEW_VERSION}."