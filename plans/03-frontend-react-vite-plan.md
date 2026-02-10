# Frontend Plan (React + Vite)

## Tech Choices

- React + Vite + TypeScript
- Package manager/runtime: Bun
- Tailwind CSS (or CSS modules) with custom dark theme tokens
- React Query for server state
- Zustand (optional) for local UI state

## App Structure

- `src/app/` application shell and providers
- `src/features/auth/` Google sign-in and session bootstrap (final rollout phase)
- `src/features/chat/` chat thread, composer, message list
- `src/features/models/` model selector
- `src/features/reasoning/` thinking-level selector and preset state
- `src/features/files/` attachment uploader and chips
- `src/features/search/` mode toggles and search activity UI

## Core UI Components

1. Sidebar:
   - New chat
   - Conversation history
2. Top bar:
   - User/account menu (sign out)
   - Model selector
   - Thinking-level selector (reasoning effort) for selected model
   - Pricing and context-window visibility for selected model
   - Grounding toggle (default ON)
   - Deep research toggle
3. Message area:
   - User/assistant bubbles
   - Streaming tokens
   - Citation cards/links
4. Composer:
   - Multiline input
   - Attach file button
   - Send button

## Implementation Tasks

1. Scaffold Vite + TypeScript + styling baseline
2. Build static layout for desktop + responsive mobile
3. Add API client with typed contracts
4. Add SSE streaming response handling
5. Add model list fetch with curated-first + show-all behavior
6. Add favorite-model actions and filter
7. Implement model defaults:
   - normal chat uses last-used model
   - first app run uses `openrouter/free`
   - deep research model defaults to last-used normal-chat model, but is user-selectable
8. Show model pricing/context window in selector and details panel
9. Add file upload flow + selected attachments preview
10. Render citations and tool activity timeline
11. Add optimistic conversation updates and retries
12. Add chat deletion UX: delete single conversation and delete all
13. Add reasoning-effort control UX:
   - model-aware enable/disable using capability metadata
   - persisted per-model + per-mode presets
   - per-send override in request payload
14. Final rollout: integrate Google Identity Services sign-in page
15. Final rollout: add auth bootstrap (`/v1/auth/me`) and guarded routes

## UI/Design Direction

- Dark-only palette with high contrast text
- Minimal chrome, generous spacing, smooth streaming animation
- Clear visual differentiation between normal and deep research mode
- Login screen follows same dark minimal visual style (added during final auth rollout)

## Acceptance Criteria

- New conversation can be created and persisted
- Model can be changed before sending a message
- Reasoning effort can be changed before sending a message
- Model metadata (pricing/context) is visible
- Reasoning control is only shown/enabled when selected model supports reasoning parameters
- Curated list can be empty while "show all models" still works
- Responses stream without UI freezes
- Files can be attached and included in request payload
- Grounding defaults ON for every message unless user disables it
- User can delete one chat or all chats from UI
- Final rollout gate: unauthenticated users cannot access chat routes and allowlisted Google login/logout works end-to-end
