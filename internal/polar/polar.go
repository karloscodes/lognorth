package polar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const (
	maxRetries       = 3
	initialRetryWait = 200 * time.Millisecond
	maxRetryWait     = 1 * time.Second

	ProductionOrgID = "6fdc5c60-15e4-4427-9e4b-f59030977543"
	SandboxOrgID    = "8b2ed80f-f53a-4340-8d3b-67678e882384"
)

// Base URLs are vars (not consts) so tests can point the client at an
// httptest server or an unreachable address. Don't reassign in production.
var (
	ProductionBaseURL = "https://api.polar.sh/v1"
	SandboxBaseURL    = "https://sandbox-api.polar.sh/v1"
)

type Client struct {
	httpClient *http.Client
	BaseURL    string
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		BaseURL:    ProductionBaseURL,
	}
}

type ValidateRequest struct {
	Key            string `json:"key"`
	OrganizationID string `json:"organization_id"`
}

type ValidateResponse struct {
	ID        string     `json:"id"`
	Customer  Customer   `json:"customer"`
	Key       string     `json:"key"`
	Status    string     `json:"status"` // granted, revoked, disabled
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type Customer struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ValidateLicense checks a license key against the Polar API.
func (c *Client) ValidateLicense(ctx context.Context, organizationID, licenseKey string) (*ValidateResponse, error) {
	if organizationID == "" || licenseKey == "" {
		return nil, fmt.Errorf("organizationID and licenseKey are required")
	}

	validateURL := c.BaseURL + "/customer-portal/license-keys/validate"
	currentWait := initialRetryWait
	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(currentWait))) - (currentWait / 2)
			waitTime := max(0, currentWait+jitter)
			time.Sleep(waitTime)
			currentWait = min(currentWait*2, maxRetryWait)
		}

		bodyBytes, _ := json.Marshal(ValidateRequest{
			Key:            licenseKey,
			OrganizationID: organizationID,
		})

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, validateURL, strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		var doErr error
		resp, doErr = c.httpClient.Do(req)
		if doErr != nil {
			lastErr = fmt.Errorf("attempt %d: request failed: %w", attempt, doErr)
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("attempt %d: server error: status %d", attempt, resp.StatusCode)
			resp.Body.Close()
			continue
		}

		if resp.StatusCode == 404 {
			resp.Body.Close()
			return nil, nil // not found
		}

		if resp.StatusCode >= 400 {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("polar error: status %d, body: %s", resp.StatusCode, string(body))
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result ValidateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// LicenseStatus returns "valid", "invalid", or "expired" based on the response.
func LicenseStatus(resp *ValidateResponse) string {
	if resp == nil || resp.ID == "" {
		return "invalid"
	}
	if resp.Status == "revoked" || resp.Status == "disabled" {
		return "invalid"
	}
	if resp.ExpiresAt != nil && resp.ExpiresAt.Before(time.Now()) {
		return "expired"
	}
	return "valid"
}
