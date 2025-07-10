#!/bin/bash

set -e

RELEASE_TYPE="$1"

if [[ "$RELEASE_TYPE" != "beta" && "$RELEASE_TYPE" != "stable" ]]; then
    echo "Usage: $0 {beta|stable}"
    exit 1
fi

# Get all tags and sort them
git fetch --tags >/dev/null 2>&1 || true

if [[ "$RELEASE_TYPE" == "beta" ]]; then
    # For beta releases, auto-generate version
    
    # Get the last beta version for the current base version
    LAST_BETA=$(git tag -l --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+-beta\.[0-9]+$' | head -1 || echo "")
    
    # Get the last stable version
    LAST_STABLE=$(git tag -l --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || echo "v0.0.0")
    
    if [[ -n "$LAST_BETA" ]]; then
        # Extract base version and beta number from last beta
        BETA_BASE=$(echo "$LAST_BETA" | sed 's/-beta\.[0-9]*$//')
        BETA_NUM=$(echo "$LAST_BETA" | sed 's/.*-beta\.//')
        
        # Compare beta base with last stable
        if [[ "$BETA_BASE" == "$LAST_STABLE" ]]; then
            # Increment beta number
            NEW_BETA_NUM=$((BETA_NUM + 1))
            NEW_VERSION="${BETA_BASE}-beta.${NEW_BETA_NUM}"
        else
            # Last stable is newer, start new beta series
            STABLE_PARTS=($(echo "${LAST_STABLE#v}" | tr '.' ' '))
            MAJOR=${STABLE_PARTS[0]}
            MINOR=${STABLE_PARTS[1]}
            PATCH=$((${STABLE_PARTS[2]} + 1))
            NEW_VERSION="v${MAJOR}.${MINOR}.${PATCH}-beta.1"
        fi
    else
        # No beta versions exist, create first one based on stable
        STABLE_PARTS=($(echo "${LAST_STABLE#v}" | tr '.' ' '))
        MAJOR=${STABLE_PARTS[0]}
        MINOR=${STABLE_PARTS[1]}
        PATCH=$((${STABLE_PARTS[2]} + 1))
        NEW_VERSION="v${MAJOR}.${MINOR}.${PATCH}-beta.1"
    fi
    
elif [[ "$RELEASE_TYPE" == "stable" ]]; then
    # For stable releases, the version is provided via workflow input
    # This script is used for validation only
    echo "Stable version should be provided via workflow input"
    exit 1
fi

# Validate that the version doesn't already exist
if git tag -l | grep -q "^${NEW_VERSION}$"; then
    echo "Error: Version ${NEW_VERSION} already exists"
    exit 1
fi

echo "${NEW_VERSION}"