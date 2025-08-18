package metadata

import (
	"context"
	"time"

	"go.uber.org/zap"
	"github.com/strettch/sc-metrics-agent/pkg/config"
)

// AuthManager handles periodic token refresh for metadata service authentication.
type AuthManager struct {
	client        *Client
	vmID          string
	logger        *zap.Logger
	refreshTicker *time.Ticker
	stopCh        chan struct{}
	currentToken  string
}

// NewAuthManager creates a new auth manager.
func NewAuthManager(cfg *config.Config, logger *zap.Logger) *AuthManager {
	client := NewClient(cfg.MetadataServiceEndpoint, cfg.HTTPTimeout, logger)
	return &AuthManager{
		client:        client,
		vmID:          cfg.VMID,
		logger:        logger,
		stopCh:        make(chan struct{}),
		refreshTicker: time.NewTicker(client.tokenLifetime),
	}
}

// IsRefreshRunning checks if the background refresh loop is running.
func (am *AuthManager) IsRefreshRunning() bool {
	return am.refreshTicker != nil
}

// fetchAndStoreToken fetches a token and stores it internally.
func (am *AuthManager) fetchAndStoreToken(ctx context.Context, forceFetch bool) error {
	if forceFetch {
		am.logger.Debug("Forcing token refresh")
		am.client.InvalidateCache()
	} else {
		am.logger.Debug("Fetching token (using cache if valid)")
	}
	token, err := am.client.GetAuthToken(ctx, am.vmID)
	if err != nil {
		return err
	}
	am.currentToken = token
	am.logger.Debug("Token stored successfully")
	return nil
}

// EnsureValidToken fetches a valid token and stores it internally.
func (am *AuthManager) EnsureValidToken(ctx context.Context) error {
	return am.fetchAndStoreToken(ctx, false)
}

// StartRefresh starts automatic token refresh.
func (am *AuthManager) StartRefresh(ctx context.Context) {
	go func() {
		for {
			select {
			case <-am.refreshTicker.C:
				if err := am.refresh(ctx); err != nil {
					am.logger.Error("Background token refresh failed", zap.Error(err))
				}
			case <-ctx.Done():
				am.logger.Info("Token refresh stopped due to context cancel")
				return
			case <-am.stopCh:
				am.logger.Info("Token refresh stopped")
				return
			}
		}
	}()
}

// refresh invalidates the cache and fetches a new token.
func (am *AuthManager) refresh(ctx context.Context) error {
	return am.fetchAndStoreToken(ctx, true)
}

// GetCurrentToken returns the current authentication token.
func (am *AuthManager) GetCurrentToken() string {
	return am.currentToken
}

// Close stops the refresh loop.
func (am *AuthManager) Close() {
	if am.refreshTicker != nil {
		am.refreshTicker.Stop()
	}
	close(am.stopCh)
}