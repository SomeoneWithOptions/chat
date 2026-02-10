# Data, Files, and Storage Plan (Turso + Optional GCS)

## Turso Schema (Draft)

### `users`

- `id` (text pk)
- `google_sub` (text unique)
- `email` (text unique)
- `name` (text nullable)
- `avatar_url` (text nullable)
- `created_at` (datetime)
- `last_login_at` (datetime)

### `sessions`

- `id` (text pk)
- `user_id` (text fk)
- `expires_at` (datetime)
- `created_at` (datetime)
- `revoked_at` (datetime nullable)

Session policy: 7-day TTL.

### `conversations`

- `id` (text pk)
- `user_id` (text fk)
- `title` (text)
- `created_at` (datetime)
- `updated_at` (datetime)
- `default_model_id` (text)

### `messages`

- `id` (text pk)
- `conversation_id` (text fk)
- `role` (`user` | `assistant` | `system`)
- `content` (text)
- `mode` (`chat` | `deep_research`)
- `grounding_enabled` (bool)
- `model_id` (text)
- `token_input` (int nullable)
- `token_output` (int nullable)
- `created_at` (datetime)

### `message_citations`

- `id` (text pk)
- `message_id` (text fk)
- `url` (text)
- `title` (text)
- `snippet` (text)
- `rank` (int)

### `files`

- `id` (text pk)
- `user_id` (text fk)
- `conversation_id` (text fk)
- `uploader_ref` (text nullable)
- `name` (text)
- `mime_type` (text)
- `size_bytes` (int)
- `storage_provider` (`local` | `gcs`)
- `storage_key` (text)
- `text_extracted` (text nullable)
- `created_at` (datetime)

### `user_model_preferences`

- `user_id` (text pk/fk)
- `favorite_model_ids` (text json)
- `last_model_id` (text nullable)
- `last_deep_research_model_id` (text nullable)
- `updated_at` (datetime)

### `models` (Capability Additions)

- `supported_parameters_json` (text nullable, raw provider-supported parameters snapshot)
- `supports_reasoning` (bool, derived from provider metadata)

### `user_model_reasoning_presets`

- `user_id` (text fk)
- `model_id` (text fk)
- `mode` (`chat` | `deep_research`)
- `effort` (`none` | `low` | `medium` | `high`)
- `updated_at` (datetime)
- Primary key: (`user_id`, `model_id`, `mode`)

## File Handling Strategy

1. Small files MVP path:
   - Upload to backend
   - Store temporary file in local processing path
   - Extract text directly
   - Persist extracted text and metadata in Turso
2. Scalable path:
   - Store blobs in GCS bucket
   - Keep metadata and extracted text in Turso

## Deletion Policy

- Chat deletions are hard deletes (no soft-delete flag).
- Deleting one chat removes dependent messages/citations/files rows.
- Deleting all chats removes all conversation-scoped rows for the user.
- If `storage_provider = gcs`, backend deletes corresponding objects from GCS.
- If `storage_provider = local`, backend does not delete original local user files.

## Allowed File Types (MVP Recommendation)

- `.txt`, `.md`, `.pdf`, `.csv`, `.json`
- Optional later: `.docx`, `.xlsx`, images with OCR

## Limits (Initial Defaults)

- Max file size: 25 MB
- Max files per message: 5
- Max extracted text per file into prompt context: configurable cap
- Reasoning presets are validated against model capability metadata before persistence

## Acceptance Criteria

- All conversation/message/file data is user-owned and query-scoped by `user_id`
- Uploaded files are linked to conversations and messages
- File extraction failures are surfaced without crashing chat request
- Attachments can be referenced in grounded/deep-research outputs
- User can delete one conversation or delete all conversations
- Per-model reasoning presets are persisted per mode and survive app reloads
