# Frontend Plan (React + Vite)

## Tech Choices

- React + Vite + TypeScript
- Package manager/runtime: Bun
- Tailwind CSS (or CSS modules) with custom dark theme tokens
- React Query for server state
- Zustand (optional) for local UI state

## App Structure

- `src/app/` application shell and providers
- `src/features/auth/` Google sign-in and session bootstrap
- `src/features/chat/` chat thread, composer, message list
- `src/features/models/` model selector
- `src/features/files/` attachment uploader and chips
- `src/features/search/` mode toggles and search activity UI

## Core UI Components

1. Sidebar:
   - New chat
   - Conversation history
2. Top bar:
   - User/account menu (sign out)
   - Model selector
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
2. Integrate Google Identity Services sign-in page
3. Add auth bootstrap (`/v1/auth/me`) and guarded routes
4. Build static layout for desktop + responsive mobile
5. Add API client with typed contracts
6. Add SSE streaming response handling
7. Add model list fetch with curated-first + show-all behavior
8. Add favorite-model actions and filter
9. Implement model defaults:
   - normal chat uses last-used model
   - first app run uses `openrouter/free`
   - deep research model defaults to last-used normal-chat model, but is user-selectable
10. Show model pricing/context window in selector and details panel
11. Add file upload flow + selected attachments preview
12. Render citations and tool activity timeline
13. Add optimistic conversation updates and retries
14. Add chat deletion UX: delete single conversation and delete all

## UI/Design Direction

- Dark-only palette with high contrast text
- Minimal chrome, generous spacing, smooth streaming animation
- Clear visual differentiation between normal and deep research mode
- Login screen follows same dark minimal visual style

## Acceptance Criteria

- New conversation can be created and persisted
- Unauthenticated users cannot access chat routes
- Allowed Google account can login/logout successfully
- App supports the configured allowlist and can expand to more emails later
- Model can be changed before sending a message
- Model metadata (pricing/context) is visible
- Curated list can be empty while "show all models" still works
- Responses stream without UI freezes
- Files can be attached and included in request payload
- Grounding defaults ON for every message unless user disables it
- User can delete one chat or all chats from UI
