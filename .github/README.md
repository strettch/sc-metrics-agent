# CI/CD Pipeline Documentation

This directory contains the GitHub Actions workflows and scripts for automated testing, building, and deployment of the SC Metrics Agent.

## Workflows

### 1. PR Validation (`.github/workflows/pr.yml`)
**Triggers:** Pull requests to `main` branch

**Actions:**
- Run unit tests
- Perform linting with golangci-lint
- Build binary to verify compilation
- Run security scans with Gosec

### 2. Beta Release (`.github/workflows/beta.yml`)
**Triggers:** Push to `main` branch

**Actions:**
- Auto-generate beta version (e.g., `0.1.0-beta.1`)
- Build and package Debian package
- Deploy to beta APT repository
- Create GitHub pre-release
- Generate commit-based changelog

### 3. Stable Release (`.github/workflows/release.yml`)
**Triggers:** Manual workflow dispatch

**Actions:**
- Validate semantic version input
- Build and package Debian package
- Deploy to stable APT repository
- Create GitHub release
- Generate comprehensive changelog

## Versioning Strategy

### Beta Versions
- **Format:** `X.Y.Z-beta.N` (e.g., `0.1.0-beta.1`)
- **Generation:** Automatic based on last release
- **Increment:** Beta number increases for each push to main

### Stable Versions
- **Format:** `X.Y.Z` (e.g., `0.1.0`)
- **Generation:** Manual input via workflow dispatch
- **Types:** major, minor, patch (validated against last stable)

## Repository Structure

The CI/CD pipeline uses your existing `setup_repo.sh` script for deployment:
- **Repository:** `https://repo.cloud.strettch.dev/`
- **Install:** `curl -sSL https://repo.cloud.strettch.dev/install.sh | sudo bash`
- **Both beta and stable releases use the same repository**

## Required Secrets

The following GitHub secrets must be configured:

- `REPO_SSH_KEY`: Base64-encoded SSH private key for repository server access
- `REPO_HOST`: Repository server hostname
- `REPO_USER`: Username for repository server access

## Server Setup Requirements

The repository server must have the following setup:

1. **GPG Passphrase File** (to fix automation):
   ```bash
   echo "your_gpg_passphrase" > /root/gpg-passphrase.txt
   chmod 600 /root/gpg-passphrase.txt
   ```

2. **Project Directory**:
   ```bash
   cd /root/sc-metrics-agent
   # The existing setup_repo.sh script must be present here
   ```

3. **Dependencies**: All dependencies for `setup_repo.sh` (aptly, gpg, etc.)

## Scripts

### `generate-version.sh`
Generates appropriate version numbers for beta releases based on Git tags.

### `generate-changelog.sh`
Creates changelogs for releases:
- **Beta:** Commits since last beta/stable
- **Stable:** Comprehensive changelog since last stable

### `deploy-to-repo.sh`
Simplified deployment script:
- Uploads package to repository server at `/root/sc-metrics-agent/`
- Runs your existing `./setup_repo.sh` script
- Uses the same workflow you're familiar with

## Usage

### Creating a Beta Release
1. Push changes to `main` branch
2. GitHub Actions automatically creates beta release
3. Version is auto-generated (e.g., `0.1.0-beta.1`)
4. Package deployed to beta repository

### Creating a Stable Release
1. Go to GitHub Actions
2. Run "Stable Release" workflow
3. Input desired version (e.g., `0.1.0`)
4. Select release type (major/minor/patch)
5. Package deployed to stable repository

### Testing a PR
1. Create pull request to `main` branch
2. GitHub Actions automatically runs tests and builds
3. Review results before merging

## Deployment Flow

```
PR → Tests/Lint/Build → Merge to main → Beta Release → Manual Stable Release
```

Each step ensures code quality and provides automated deployment to appropriate repositories.