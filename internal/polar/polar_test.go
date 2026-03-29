package polar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLicenseStatus(t *testing.T) {
	t.Run("nil response is invalid", func(t *testing.T) {
		status := LicenseStatus(nil)

		if status != "invalid" {
			t.Errorf("expected invalid, got %s", status)
		}
	})

	t.Run("empty ID is invalid", func(t *testing.T) {
		resp := &ValidateResponse{ID: "", Status: "granted"}

		status := LicenseStatus(resp)

		if status != "invalid" {
			t.Errorf("expected invalid, got %s", status)
		}
	})

	t.Run("granted status is valid", func(t *testing.T) {
		resp := &ValidateResponse{ID: "abc123", Status: "granted"}

		status := LicenseStatus(resp)

		if status != "valid" {
			t.Errorf("expected valid, got %s", status)
		}
	})

	t.Run("revoked status is invalid", func(t *testing.T) {
		resp := &ValidateResponse{ID: "abc123", Status: "revoked"}

		status := LicenseStatus(resp)

		if status != "invalid" {
			t.Errorf("expected invalid, got %s", status)
		}
	})

	t.Run("disabled status is invalid", func(t *testing.T) {
		resp := &ValidateResponse{ID: "abc123", Status: "disabled"}

		status := LicenseStatus(resp)

		if status != "invalid" {
			t.Errorf("expected invalid, got %s", status)
		}
	})

	t.Run("expired license is expired", func(t *testing.T) {
		past := time.Now().Add(-24 * time.Hour)
		resp := &ValidateResponse{ID: "abc123", Status: "granted", ExpiresAt: &past}

		status := LicenseStatus(resp)

		if status != "expired" {
			t.Errorf("expected expired, got %s", status)
		}
	})

	t.Run("future expiry is valid", func(t *testing.T) {
		future := time.Now().Add(365 * 24 * time.Hour)
		resp := &ValidateResponse{ID: "abc123", Status: "granted", ExpiresAt: &future}

		status := LicenseStatus(resp)

		if status != "valid" {
			t.Errorf("expected valid, got %s", status)
		}
	})

	t.Run("no expiry is valid", func(t *testing.T) {
		resp := &ValidateResponse{ID: "abc123", Status: "granted", ExpiresAt: nil}

		status := LicenseStatus(resp)

		if status != "valid" {
			t.Errorf("expected valid, got %s", status)
		}
	})
}

func TestValidateLicense(t *testing.T) {
	t.Run("valid license key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/customer-portal/license-keys/validate" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var req ValidateRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Key != "test-key-123" {
				t.Errorf("expected key test-key-123, got %s", req.Key)
			}
			if req.OrganizationID != "org-123" {
				t.Errorf("expected org org-123, got %s", req.OrganizationID)
			}

			json.NewEncoder(w).Encode(ValidateResponse{
				ID:     "license-id",
				Status: "granted",
				Customer: Customer{
					Email: "buyer@example.com",
					Name:  "Test User",
				},
			})
		}))
		defer server.Close()

		client := &Client{httpClient: server.Client(), BaseURL: server.URL}

		resp, err := client.ValidateLicense(context.Background(), "org-123", "test-key-123")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ID != "license-id" {
			t.Errorf("expected license-id, got %s", resp.ID)
		}
		if resp.Customer.Email != "buyer@example.com" {
			t.Errorf("expected buyer@example.com, got %s", resp.Customer.Email)
		}
		if LicenseStatus(resp) != "valid" {
			t.Errorf("expected valid status")
		}
	})

	t.Run("license not found returns nil", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := &Client{httpClient: server.Client(), BaseURL: server.URL}

		resp, err := client.ValidateLicense(context.Background(), "org-123", "bad-key")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != nil {
			t.Error("expected nil response for not found")
		}
	})

	t.Run("400 error returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"detail":"invalid request"}`))
		}))
		defer server.Close()

		client := &Client{httpClient: server.Client(), BaseURL: server.URL}

		_, err := client.ValidateLicense(context.Background(), "org-123", "test-key")

		if err == nil {
			t.Fatal("expected error for 400 response")
		}
	})

	t.Run("empty org or key returns error", func(t *testing.T) {
		client := NewClient()

		_, err := client.ValidateLicense(context.Background(), "", "key")
		if err == nil {
			t.Error("expected error for empty org")
		}

		_, err = client.ValidateLicense(context.Background(), "org", "")
		if err == nil {
			t.Error("expected error for empty key")
		}
	})

	t.Run("server error retries", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts <= 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(ValidateResponse{
				ID:     "license-id",
				Status: "granted",
			})
		}))
		defer server.Close()

		client := &Client{httpClient: server.Client(), BaseURL: server.URL}

		resp, err := client.ValidateLicense(context.Background(), "org-123", "test-key")

		if err != nil {
			t.Fatalf("unexpected error after retries: %v", err)
		}
		if resp.ID != "license-id" {
			t.Errorf("expected license-id, got %s", resp.ID)
		}
		if attempts < 3 {
			t.Errorf("expected at least 3 attempts, got %d", attempts)
		}
	})
}
