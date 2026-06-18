package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
	_ "modernc.org/sqlite"
	"github.com/MileniumTick/aimux/internal/infrastructure/sqlite"
)

func TestFetchModels_OpenAIFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("expected /v1/models, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept: application/json, got %s", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != "aimux/0.1.0" {
			t.Errorf("expected User-Agent: aimux/0.1.0, got %s", r.Header.Get("User-Agent"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"},
			},
		})
	}))
	defer srv.Close()

	uc := setupProviderTest(t)
	id, err := uc.Add("TestOpenAI", srv.URL, "api-key", "test-token", domain.ApiTypeOpenAI)
	if err != nil {
		t.Fatalf("Add provider failed: %v", err)
	}

	// Verify models were stored
	models, err := uc.providerRepo.ListModels(id)
	if err != nil {
		t.Fatalf("ListModels after Add failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ModelName != "gpt-4o" && models[0].ModelName != "gpt-4o-mini" {
		t.Errorf("unexpected model name: %q", models[0].ModelName)
	}
}

func TestFetchModels_AnthropicFallbackFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{
				{"id": "claude-sonnet-4-20250514"},
				{"id": "claude-haiku-3-20250313"},
			},
		})
	}))
	defer srv.Close()

	uc := setupProviderTest(t)
	id, err := uc.Add("TestAnthropic", srv.URL, "api-key", "test-token", domain.ApiTypeOpenAI)
	if err != nil {
		t.Fatalf("Add provider failed: %v", err)
	}

	models, _ := uc.providerRepo.ListModels(id)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
}

func TestFetchModels_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer srv.Close()

	uc := setupProviderTest(t)

	id, err := uc.providerRepo.Add("TestUnauthorized", srv.URL, "api-key", "test-token", domain.ApiTypeOpenAI)
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	err = uc.FetchModels(id, srv.URL, "test-token", domain.ApiTypeOpenAI)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if err.Error() != "authentication failed: check auth token" {
		t.Errorf("expected auth error message, got %q", err.Error())
	}
}

func TestFetchModels_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	uc := setupProviderTest(t)
	id, _ := uc.providerRepo.Add("TestRateLimit", srv.URL, "api-key", "test-token", domain.ApiTypeOpenAI)

	err := uc.FetchModels(id, srv.URL, "test-token", domain.ApiTypeOpenAI)
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}
	if err.Error() != "rate limited by provider" {
		t.Errorf("expected 'rate limited' message, got %q", err.Error())
	}
}

func TestFetchModels_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	uc := setupProviderTest(t)
	id, _ := uc.providerRepo.Add("TestServerError", srv.URL, "api-key", "test-token", domain.ApiTypeOpenAI)

	err := uc.FetchModels(id, srv.URL, "test-token", domain.ApiTypeOpenAI)
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestFetchModels_UnparseableResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	uc := setupProviderTest(t)
	id, _ := uc.providerRepo.Add("TestUnparseable", srv.URL, "api-key", "test-token", domain.ApiTypeOpenAI)

	err := uc.FetchModels(id, srv.URL, "test-token", domain.ApiTypeOpenAI)
	if err == nil {
		t.Fatal("expected error for unparseable response, got nil")
	}
}

func TestRetryFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "claude-sonnet-4"},
			},
		})
	}))
	defer srv.Close()

	uc := setupProviderTest(t)
	id, err := uc.Add("TestRetry", srv.URL, "api-key", "test-token", domain.ApiTypeOpenAI)
	if err != nil {
		t.Fatalf("Add provider failed: %v", err)
	}

	// Reset status to error
	uc.providerRepo.UpdateStatus(id, "error")

	// Retry fetch
	if _, err := uc.RetryFetch(id); err != nil {
		t.Fatalf("RetryFetch failed: %v", err)
	}

	provider, _ := uc.Get(id)
	if provider.Status != "active" {
		t.Errorf("expected status 'active' after retry, got %q", provider.Status)
	}
}

func TestRetryFetch_Failure(t *testing.T) {
	uc := setupProviderTest(t)
	id, _ := uc.providerRepo.Add("RetryFail", "http://localhost:19999", "api-key", "bad-token", domain.ApiTypeOpenAI)
	uc.providerRepo.UpdateStatus(id, "error")

	_, err := uc.RetryFetch(id)
	if err == nil {
		t.Log("RetryFetch succeeded (unexpected)")
	} else {
		provider, _ := uc.Get(id)
		if provider.Status != "error" {
			t.Errorf("expected status 'error' after failed retry, got %q", provider.Status)
		}
	}
}

func TestFetchModels_NoResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	uc := setupProviderTest(t)
	id, _ := uc.providerRepo.Add("EmptyBody", srv.URL, "api-key", "token", domain.ApiTypeOpenAI)

	err := uc.FetchModels(id, srv.URL, "token", domain.ApiTypeOpenAI)
	if err == nil {
		t.Fatal("expected error for empty response with no model arrays, got nil")
	}
}

func TestProviderRepo_AddDuplicate(t *testing.T) {
	db := setupTestDB(t)
	providerRepo := &sqlite.ProviderRepository{DB: db}

	_, err := providerRepo.Add("Duplicate", "https://api.test.com", "key1", "token1", domain.ApiTypeOpenAI)
	if err != nil {
		t.Fatalf("first Add failed: %v", err)
	}

	_, err = providerRepo.Add("Duplicate", "https://api.test.com", "key2", "token2", domain.ApiTypeOpenAI)
	if err == nil {
		t.Fatal("expected error for duplicate provider name, got nil")
	}
}
