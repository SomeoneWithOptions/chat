# Product Scope and MVP

## Product Goal

Build a minimal, modern, dark-mode-only chat interface (inspired by T3 Chat) where users can:

- Authenticate with Google before accessing the app
- Select an LLM model
- Chat with streaming responses
- Attach files to chats
- Use web grounding by default
- Switch to a deeper research mode for richer analysis

## MVP Features

1. Chat UI with conversation list + active thread
2. Google sign-in gate (allowlisted emails, starting with 2 users)
3. Model selector (OpenRouter-backed)
4. Curated/favorites model picker with optional "show all models"
5. Streaming assistant responses
6. File attachment upload + send with prompt
7. Grounding toggle (default: ON)
8. "Deep Research" mode toggle
9. Message citations for web-grounded responses
10. Persisted chat history in Turso
11. Delete controls: single chat and delete-all chats

## Post-MVP Features

- Role-based access and team spaces
- Shared/public chats
- Rich document parsing pipeline (PDF OCR, DOCX tables)
- Agentic multi-step tools beyond web search

## Non-Goals (Initial Build)

- Fine-tuning custom models
- Self-hosted model inference
- Mobile native apps

## UX Requirements

- Dark mode only
- Fast startup and minimal UI chrome
- Streaming output with visible tool/search activity
- Unauthenticated visitors are redirected to Google sign-in
- Clear mode distinction:
  - `Chat` (normal search depth)
  - `Deep Research` (longer and more citations)
- Model selection behavior:
  - use last-used model for normal chat
  - if first use, default to `openrouter/free`
  - deep research model is selectable and defaults to last-used normal model
  - curated model list can start empty with "show all" available

## Success Criteria

- End-to-end chat response under 5s p50 (excluding deep research mode)
- Deep research completion under 120s timeout for standard queries
- File attachment success rate > 99% for allowed types
- Authenticated session restoration works after browser refresh
- No unhandled backend errors for core chat flow in staged testing
