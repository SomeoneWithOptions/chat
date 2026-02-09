# Testing, Security, and Observability Plan

## Testing Strategy

### Frontend

- Unit tests for core UI logic/components
- Integration tests for chat send/stream/render flow
- Integration tests for auth guard + login bootstrap
- E2E smoke test for full chat lifecycle

### Backend

- Unit tests for prompt orchestration, search workflow, citation builder
- API tests for request validation and error envelopes
- Auth tests for Google token verification and allowlist enforcement
- Deletion tests for hard-delete behavior and GCS object cleanup paths
- Provider integration tests (mock + optional live sanity checks)

## Security Baseline

- API key secrets only in platform secret stores
- Session cookies are HTTP-only, secure, and same-site constrained
- Server-side Google ID token verification required on login
- Email allowlist via configuration (initially `acastesol@gmail.com` and `obzen.black@gmail.com`)
- Input validation and size limits on all endpoints
- MIME and extension checks for attachments
- Optional malware scan step for uploaded files (future hardening)
- No API keys or sensitive content logged
- Basic abuse controls (IP/session throttling) without user cost caps in MVP

## Observability

- Structured JSON logs
- Request IDs propagated frontend -> backend
- Metrics:
  - latency by endpoint
  - OpenRouter call duration/errors
  - Brave call duration/errors
  - token estimates and cost buckets

## Operational Alerts

- Elevated 5xx rate
- Unauthorized request spikes
- Research timeout spikes
- Provider failure rate threshold
- Attachment upload failures threshold

## Acceptance Criteria

- Core user journeys have automated tests
- Authenticated access control is enforced across all API routes
- Logs and metrics are sufficient to debug production incidents
- Basic abuse and input protections are in place before launch
- Session expiry and refresh behavior is verified for 7-day login window
- Delete-one/delete-all operations remove DB rows and cleanup GCS attachments when applicable
