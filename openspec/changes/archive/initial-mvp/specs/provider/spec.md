# Provider — HTTP Model Fetching Spec

## Scope

The HTTP client that fetches `/v1/models` from registered providers, parses the response, and returns raw model ID strings. This is a passive fetch — no classification, no ranking, no validation of model capabilities.

## HTTP Client Behavior

### Endpoint

- MUST GET `{provider_base_url}/v1/models`.
- The base URL is exactly as stored in the `providers` table (e.g., `https://api.openai.com/v1` yields request to `https://api.openai.com/v1/v1/models` — the schema stores full base URL including `/v1` if the provider's API requires it; if not, the user enters the bare base).

NOTE: The proposal says `GET /v1/models` from the provider. The base URL is what the user provides. The caller MUST append `/v1/models` to the user-provided base URL. So if the user provides `https://api.anthropic.com`, the request goes to `https://api.anthropic.com/v1/models`.

### Authentication

- MUST send the `auth_token` as a Bearer token in the Authorization header: `Authorization: Bearer {auth_token}`.
- The `api_key` is stored but NOT used in the model fetch request. It is reserved for the profile switch (injected into the target CLI's `env` block if the target CLI requires an `ANTHROPIC_API_KEY`-type variable).

### Headers

| Header | Value |
|--------|-------|
| Authorization | `Bearer {auth_token}` |
| Accept | `application/json` |
| User-Agent | `aimux/0.1.0` |

### Timeout

- The HTTP client MUST have a 5-second timeout for the entire request (dial + TLS + response body).
- If the timeout is exceeded, the fetch MUST be treated as a failure (same as HTTP error).
- The timeout is configured via `http.Client.Timeout`.

### Response Parsing

- The response body MUST be parsed as JSON.
- The parser MUST support the OpenAI-compatible `/v1/models` response format:
  ```json
  {
    "data": [
      {"id": "gpt-4o", "object": "model", ...},
      {"id": "gpt-4o-mini", ...}
    ]
  }
  ```
- Extracted values: the `"id"` field from each object in the `"data"` array.
- If the response body does not contain a `"data"` array, the parser MUST fall back to a top-level `"models"` array (Anthropic-compatible format fallback).
- If neither format is parseable, the fetch MUST return an error.
- All model IDs MUST be returned as raw strings with no normalization (no trimming, lowercasing, or prefix stripping).

### Error Handling

| Error Type | Behavior |
|------------|----------|
| HTTP 401/403 | Return error: "Authentication failed: check auth token" |
| HTTP 429 | Return error: "Rate limited by provider" |
| HTTP 5xx | Return error: "Provider returned server error: {status}" |
| Timeout | Return error: "Request timed out after 5 seconds" |
| Network error | Return error: "Network error: {error}" |
| Invalid JSON | Return error: "Invalid response format: {parse error}" |

- The error message MUST be surfaced in the TUI when the fetch fails and the provider is saved with `status = 'error'`.

### Retry

- Retry is initiated by the user from the TUI ("Retry Fetch" action on an error-status provider).
- The retry fetches models again using the stored `base_url` and `auth_token`.
- On retry success: `InsertModels()` (clear + re-insert), `UpdateProviderStatus(id, "active")`.
- On retry failure: status stays `"error"`, error shown again in TUI.

## Acceptance Scenarios

### Successful Fetch — OpenAI Format

Given a provider with `base_url = "https://api.openai.com"` and a valid `auth_token`  
When `/v1/models` is fetched  
And the response is `{"data": [{"id": "gpt-4o"}, {"id": "gpt-4o-mini"}]}`  
Then the parsed model IDs are `["gpt-4o", "gpt-4o-mini"]`  
And no error is returned

### Successful Fetch — Anthropic Fallback Format

Given a provider with `base_url = "https://api.anthropic.com"`  
When `/v1/models` is fetched  
And the response is `{"models": [{"id": "claude-sonnet-4-20250514"}, {"id": "claude-haiku-3-20250313"}]}`  
Then the parsed model IDs are `["claude-sonnet-4-20250514", "claude-haiku-3-20250313"]`  
And no error is returned

### Authentication Failure

Given a provider with an invalid `auth_token`  
When `/v1/models` is fetched  
And the response is HTTP 401  
Then the function returns error: "Authentication failed: check auth token"

### Timeout

Given a provider whose API endpoint does not respond within 5 seconds  
When `/v1/models` is fetched  
Then the function returns error: "Request timed out after 5 seconds"

### Unparseable Response

Given a provider that returns non-JSON or unrecognized JSON structure  
When `/v1/models` is fetched  
Then the function returns error indicating invalid response format

### Retry After Error

Given a provider has `status = 'error'`  
When the user triggers "Retry Fetch"  
And the fetch succeeds this time  
Then `InsertModels` is called with the new model list (replacing old ones)  
And `UpdateProviderStatus` sets status to `'active'`
