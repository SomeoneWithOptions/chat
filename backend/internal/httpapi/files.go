package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"rsc.io/pdf"
)

const (
	maxUploadBytes             = 25 * 1024 * 1024
	maxMultipartRequestBytes   = maxUploadBytes + (1 * 1024 * 1024)
	maxFilesPerMessage         = 5
	maxExtractedTextRunes      = 200_000
	maxPerFilePromptTextRunes  = 10_000
	maxTotalPromptTextRunes    = 30_000
	defaultObjectStoragePrefix = "chat-uploads"
)

var (
	errInvalidFileIDs      = errors.New("one or more fileIds are invalid")
	errTooManyFileIDs      = errors.New("too many fileIds")
	errUnsupportedFileType = errors.New("unsupported file type")

	supportedUploadExtensions = map[string]struct{}{
		".txt":  {},
		".md":   {},
		".pdf":  {},
		".csv":  {},
		".json": {},
	}

	filenameSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

type fileObjectStore interface {
	Backend() string
	PutObject(ctx context.Context, objectPath, contentType string, data []byte) error
	DeleteObject(ctx context.Context, objectPath string) error
}

type fileResponse struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MediaType string `json:"mediaType"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

type storedFile struct {
	ID            string
	Filename      string
	MediaType     string
	SizeBytes     int64
	ExtractedText string
}

type storedBlobRef struct {
	FileID      string
	StoragePath string
}

func (h Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	user, ok := sessionUserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid session")
		return
	}
	user, err := h.persistedSessionUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to resolve user")
		return
	}
	if h.files == nil {
		writeError(w, http.StatusServiceUnavailable, "attachments_unconfigured", "attachments storage is not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartRequestBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "file size exceeds 25 MB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "request must be multipart/form-data")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			writeError(w, http.StatusBadRequest, "invalid_request", "file field is required")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to read uploaded file")
		return
	}
	defer file.Close()

	filename := sanitizeFilename(header.Filename)
	extension := strings.ToLower(filepath.Ext(filename))
	if _, supported := supportedUploadExtensions[extension]; !supported {
		writeError(w, http.StatusBadRequest, "unsupported_file_type", "supported file types: .txt, .md, .pdf, .csv, .json")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to read uploaded file")
		return
	}
	if int64(len(data)) > maxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "file size exceeds 25 MB")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "empty files are not allowed")
		return
	}

	extractedText, err := extractUploadedText(extension, data)
	if err != nil {
		if errors.Is(err, errUnsupportedFileType) {
			writeError(w, http.StatusBadRequest, "unsupported_file_type", "supported file types: .txt, .md, .pdf, .csv, .json")
			return
		}
		writeError(w, http.StatusBadRequest, "file_extraction_failed", "failed to extract text from attachment")
		return
	}
	extractedText = trimToRunes(extractedText, maxExtractedTextRunes)
	if strings.TrimSpace(extractedText) == "" {
		writeError(w, http.StatusBadRequest, "file_extraction_failed", "attachment did not contain extractable text")
		return
	}

	mediaType := detectUploadMediaType(header.Header.Get("Content-Type"), extension, data)
	fileID := uuid.NewString()
	objectPath := h.buildObjectPath(user.ID, fileID, filename)

	if err := h.files.PutObject(r.Context(), objectPath, mediaType, data); err != nil {
		log.Printf("upload attachment object failed: user_id=%s file_id=%s err=%v", user.ID, fileID, err)
		writeError(w, http.StatusBadGateway, "storage_error", "failed to store attachment")
		return
	}

	var response fileResponse
	err = h.db.QueryRowContext(r.Context(), `
INSERT INTO files (
  id,
  user_id,
  filename,
  media_type,
  size_bytes,
  storage_backend,
  storage_path,
  extracted_text
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, filename, media_type, size_bytes, created_at;
`, fileID, user.ID, filename, mediaType, len(data), h.files.Backend(), objectPath, extractedText).Scan(
		&response.ID,
		&response.Filename,
		&response.MediaType,
		&response.SizeBytes,
		&response.CreatedAt,
	)
	if err != nil {
		log.Printf("persist attachment metadata failed: user_id=%s file_id=%s err=%v", user.ID, fileID, err)
		_ = h.files.DeleteObject(r.Context(), objectPath)
		writeError(w, http.StatusInternalServerError, "db_error", "failed to save file metadata")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"file": response})
}

func (h Handler) resolveUserFiles(ctx context.Context, userID string, requestedFileIDs []string) ([]storedFile, []string, error) {
	fileIDs := normalizeIDs(requestedFileIDs)
	if len(fileIDs) == 0 {
		return nil, nil, nil
	}
	if len(fileIDs) > maxFilesPerMessage {
		return nil, nil, errTooManyFileIDs
	}

	query, args := userFilesQuery(userID, fileIDs)
	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	filesByID := make(map[string]storedFile, len(fileIDs))
	for rows.Next() {
		var file storedFile
		if err := rows.Scan(&file.ID, &file.Filename, &file.MediaType, &file.SizeBytes, &file.ExtractedText); err != nil {
			return nil, nil, err
		}
		filesByID[file.ID] = file
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	ordered := make([]storedFile, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		file, ok := filesByID[fileID]
		if !ok {
			return nil, nil, errInvalidFileIDs
		}
		ordered = append(ordered, file)
	}

	return ordered, fileIDs, nil
}

func userFilesQuery(userID string, fileIDs []string) (string, []any) {
	args := make([]any, 0, len(fileIDs)+1)
	args = append(args, userID)
	for _, fileID := range fileIDs {
		args = append(args, fileID)
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(fileIDs)), ",")
	query := fmt.Sprintf(`
SELECT id, filename, media_type, size_bytes, extracted_text
FROM files
WHERE user_id = ? AND id IN (%s);
`, placeholders)

	return query, args
}

func (h Handler) insertUserMessageWithFiles(ctx context.Context, userID, conversationID, content, modelID string, groundingEnabled, deepResearchEnabled bool, fileIDs []string) (string, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	nullableModelID, err := resolveNullableModelID(ctx, tx, modelID)
	if err != nil {
		return "", err
	}

	messageID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages (
  id,
  conversation_id,
  user_id,
  role,
  content,
  model_id,
  grounding_enabled,
  deep_research_enabled
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, messageID, conversationID, userID, "user", content, nullableModelID, boolToInt(groundingEnabled), boolToInt(deepResearchEnabled)); err != nil {
		return "", err
	}

	if len(fileIDs) > 0 {
		if err := ensureFilesExistForUserTx(ctx, tx, userID, fileIDs); err != nil {
			return "", err
		}

		for _, fileID := range fileIDs {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO message_files (message_id, file_id)
VALUES (?, ?)
ON CONFLICT(message_id, file_id) DO NOTHING;
`, messageID, fileID); err != nil {
				return "", err
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;
`, conversationID, userID); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return messageID, nil
}

func ensureFilesExistForUserTx(ctx context.Context, tx *sql.Tx, userID string, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	args := make([]any, 0, len(fileIDs)+1)
	args = append(args, userID)
	for _, fileID := range fileIDs {
		args = append(args, fileID)
	}

	query := fmt.Sprintf(`
SELECT id
FROM files
WHERE user_id = ? AND id IN (%s);
`, strings.TrimRight(strings.Repeat("?,", len(fileIDs)), ","))

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	seen := make(map[string]struct{}, len(fileIDs))
	for rows.Next() {
		var fileID string
		if err := rows.Scan(&fileID); err != nil {
			return err
		}
		seen[fileID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, fileID := range fileIDs {
		if _, ok := seen[fileID]; !ok {
			return errInvalidFileIDs
		}
	}

	return nil
}

func (h Handler) appendFileContextToPrompt(message string, files []storedFile) string {
	baseMessage := strings.TrimSpace(message)
	if len(files) == 0 {
		return baseMessage
	}

	var builder strings.Builder
	builder.WriteString(baseMessage)
	builder.WriteString("\n\nAttached file excerpts:\n")

	remaining := maxTotalPromptTextRunes
	for i, file := range files {
		if remaining <= 0 {
			break
		}

		excerpt := strings.TrimSpace(file.ExtractedText)
		if excerpt == "" {
			continue
		}
		excerpt = trimToRunes(excerpt, maxPerFilePromptTextRunes)
		excerpt = trimToRunes(excerpt, remaining)
		if excerpt == "" {
			continue
		}

		builder.WriteString(fmt.Sprintf("\n[%d] %s (%s)\n", i+1, file.Filename, file.MediaType))
		builder.WriteString(excerpt)
		builder.WriteString("\n")
		remaining -= utf8.RuneCountInString(excerpt)
	}

	if remaining <= 0 {
		builder.WriteString("\n[Attachment text truncated to fit prompt budget.]")
	}

	return strings.TrimSpace(builder.String())
}

func (h Handler) buildObjectPath(userID, fileID, filename string) string {
	prefix := strings.Trim(strings.TrimSpace(h.cfg.GCSUploadPrefix), "/")
	if prefix == "" {
		prefix = defaultObjectStoragePrefix
	}
	return path.Join(prefix, "users", userID, fileID, filename)
}

func (h Handler) listConversationBlobRefs(ctx context.Context, userID, conversationID string) ([]storedBlobRef, error) {
	rows, err := h.db.QueryContext(ctx, `
SELECT DISTINCT f.id, f.storage_path
FROM files f
JOIN message_files mf ON mf.file_id = f.id
JOIN messages m ON m.id = mf.message_id
JOIN conversations c ON c.id = m.conversation_id
WHERE c.user_id = ? AND c.id = ? AND f.user_id = ?;
`, userID, conversationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refs := make([]storedBlobRef, 0, 8)
	for rows.Next() {
		var ref storedBlobRef
		if err := rows.Scan(&ref.FileID, &ref.StoragePath); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func (h Handler) listConversationBlobRefsFromMessageRow(ctx context.Context, userID, conversationID string, fromMessageRowID int64) ([]storedBlobRef, error) {
	rows, err := h.db.QueryContext(ctx, `
SELECT DISTINCT f.id, f.storage_path
FROM files f
JOIN message_files mf ON mf.file_id = f.id
JOIN messages m ON m.id = mf.message_id
JOIN conversations c ON c.id = m.conversation_id
WHERE c.user_id = ? AND c.id = ? AND f.user_id = ? AND m.rowid >= ?;
`, userID, conversationID, userID, fromMessageRowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refs := make([]storedBlobRef, 0, 8)
	for rows.Next() {
		var ref storedBlobRef
		if err := rows.Scan(&ref.FileID, &ref.StoragePath); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func (h Handler) listAllUserConversationBlobRefs(ctx context.Context, userID string) ([]storedBlobRef, error) {
	rows, err := h.db.QueryContext(ctx, `
SELECT DISTINCT f.id, f.storage_path
FROM files f
JOIN message_files mf ON mf.file_id = f.id
JOIN messages m ON m.id = mf.message_id
JOIN conversations c ON c.id = m.conversation_id
WHERE c.user_id = ? AND f.user_id = ?;
`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refs := make([]storedBlobRef, 0, 16)
	for rows.Next() {
		var ref storedBlobRef
		if err := rows.Scan(&ref.FileID, &ref.StoragePath); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func (h Handler) cleanupOrphanedFileBlobs(ctx context.Context, userID string, candidates []storedBlobRef) {
	if len(candidates) == 0 {
		return
	}

	unique := make(map[string]storedBlobRef, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.FileID) == "" {
			continue
		}
		unique[candidate.FileID] = candidate
	}

	for _, candidate := range unique {
		var refs int
		err := h.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM message_files
WHERE file_id = ?;
`, candidate.FileID).Scan(&refs)
		if err != nil {
			log.Printf("cleanup attachment check failed: user_id=%s file_id=%s err=%v", userID, candidate.FileID, err)
			continue
		}
		if refs > 0 {
			continue
		}

		var storageBackend string
		var storagePath string
		err = h.db.QueryRowContext(ctx, `
SELECT storage_backend, storage_path
FROM files
WHERE id = ? AND user_id = ?
LIMIT 1;
`, candidate.FileID, userID).Scan(&storageBackend, &storagePath)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			log.Printf("cleanup attachment metadata lookup failed: user_id=%s file_id=%s err=%v", userID, candidate.FileID, err)
			continue
		}

		if _, err := h.db.ExecContext(ctx, `
DELETE FROM files
WHERE id = ? AND user_id = ?;
`, candidate.FileID, userID); err != nil {
			log.Printf("cleanup attachment metadata delete failed: user_id=%s file_id=%s err=%v", userID, candidate.FileID, err)
			continue
		}

		if h.files != nil && storageBackend == "gcs" && strings.TrimSpace(storagePath) != "" {
			if err := h.files.DeleteObject(ctx, storagePath); err != nil {
				log.Printf("cleanup attachment blob delete failed: user_id=%s file_id=%s path=%s err=%v", userID, candidate.FileID, storagePath, err)
			}
		}
	}
}

func extractUploadedText(extension string, data []byte) (string, error) {
	switch extension {
	case ".txt", ".md", ".csv":
		return normalizeTextPayload(string(data)), nil
	case ".json":
		if !json.Valid(data) {
			return "", errors.New("invalid json")
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, data, "", "  "); err != nil {
			return "", err
		}
		return normalizeTextPayload(pretty.String()), nil
	case ".pdf":
		text, err := extractPDFText(data)
		if err != nil {
			return "", err
		}
		return normalizeTextPayload(text), nil
	default:
		return "", errUnsupportedFileType
	}
}

func extractPDFText(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	var textBuilder strings.Builder
	runeCount := 0
	for pageNum := 1; pageNum <= reader.NumPage(); pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		content := page.Content()
		for _, item := range content.Text {
			chunk := strings.TrimSpace(item.S)
			if chunk == "" {
				continue
			}
			if textBuilder.Len() > 0 {
				textBuilder.WriteByte('\n')
				runeCount++
			}
			textBuilder.WriteString(chunk)
			runeCount += utf8.RuneCountInString(chunk)
			if runeCount >= maxExtractedTextRunes {
				return trimToRunes(textBuilder.String(), maxExtractedTextRunes), nil
			}
		}
	}

	return textBuilder.String(), nil
}

func normalizeTextPayload(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ToValidUTF8(normalized, "")
	return strings.TrimSpace(normalized)
}

func detectUploadMediaType(headerContentType, extension string, data []byte) string {
	contentType := strings.TrimSpace(headerContentType)
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType
	}

	if byExt := strings.TrimSpace(mime.TypeByExtension(extension)); byExt != "" {
		return byExt
	}

	if len(data) > 0 {
		sniffLen := len(data)
		if sniffLen > 512 {
			sniffLen = 512
		}
		return http.DetectContentType(data[:sniffLen])
	}

	return "application/octet-stream"
}

func sanitizeFilename(raw string) string {
	base := strings.TrimSpace(filepath.Base(raw))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "file"
	}

	extension := filepath.Ext(base)
	namePart := strings.TrimSuffix(base, extension)
	namePart = filenameSanitizer.ReplaceAllString(namePart, "_")
	namePart = strings.Trim(namePart, "._")
	if namePart == "" {
		namePart = "file"
	}

	extension = strings.ToLower(extension)
	extension = filenameSanitizer.ReplaceAllString(extension, "")
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	candidate := namePart + extension
	candidate = trimToRunes(candidate, 180)
	if strings.TrimSpace(candidate) == "" {
		return "file"
	}
	return candidate
}

func normalizeIDs(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func trimToRunes(raw string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(raw) <= limit {
		return raw
	}
	return string([]rune(raw)[:limit])
}
