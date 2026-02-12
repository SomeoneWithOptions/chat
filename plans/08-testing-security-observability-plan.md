# Testing, Security, and Observability Plan

## Testing Strategy

### Frontend

- Unit tests for core UI logic/components
- Integration tests for chat send/stream/render flow
- E2E smoke test for full chat lifecycle
- UI tests for reasoning-effort selector behavior:
  - hidden/disabled when model lacks reasoning support
  - persists per model + mode
  - included in chat request payload
- Final rollout auth suite: auth guard + login bootstrap

### Backend

- Unit tests for prompt orchestration, search workflow, citation builder
- API tests for request validation and error envelopes
- Deletion tests for hard-delete behavior and GCS object cleanup paths
- Provider integration tests (mock + optional live sanity checks)
- Model sync tests for reasoning capability metadata ingestion
- Reasoning preset API tests (`PUT /v1/models/reasoning-presets`) and request-resolution precedence tests
- Final rollout auth suite: Google token verification and allowlist enforcement

## Security Baseline

- API key secrets only in platform secret stores
- Session cookies are HTTP-only, secure, and same-site constrained
- Input validation and size limits on all endpoints
- MIME and extension checks for attachments
- Optional malware scan step for uploaded files (future hardening)
- No API keys or sensitive content logged
- Basic abuse controls (IP/session throttling) without user cost caps in MVP
- Final rollout gate: server-side Google ID token verification + email allowlist enforcement

## Observability

- Structured JSON logs
- Request IDs propagated frontend -> backend
- Metrics:
  - latency by endpoint
  - OpenRouter call duration/errors
  - Brave call duration/errors
  - token estimates and cost buckets
  - reasoning-effort usage distribution by mode/model

## Operational Alerts

- Elevated 5xx rate
- Unauthorized request spikes
- Research timeout spikes
- Provider failure rate threshold
- Attachment upload failures threshold

## Acceptance Criteria

- Core user journeys have automated tests
- Logs and metrics are sufficient to debug production incidents
- Basic abuse and input protections are in place before launch
- Delete-one/delete-all operations remove DB rows and cleanup GCS attachments when applicable
- Final rollout gate: authenticated access control is enforced across all API routes, including 30-day session expiry verification
- Reasoning-effort controls are reliable under unsupported-model, malformed-effort, and provider-error scenarios
