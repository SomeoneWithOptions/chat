package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/googleapi"
	gcsapi "google.golang.org/api/storage/v1"
)

type gcsObjectStore struct {
	bucketName string
	service    *gcsapi.Service
}

func newGCSObjectStore(ctx context.Context, bucketName string) (*gcsObjectStore, error) {
	trimmedBucket := strings.TrimSpace(bucketName)
	if trimmedBucket == "" {
		return nil, errors.New("gcs bucket is required")
	}

	service, err := gcsapi.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gcs service: %w", err)
	}

	if _, err := service.Buckets.Get(trimmedBucket).Context(ctx).Do(); err != nil {
		return nil, fmt.Errorf("read gcs bucket attrs: %w", err)
	}

	return &gcsObjectStore{bucketName: trimmedBucket, service: service}, nil
}

func (s *gcsObjectStore) Backend() string {
	return "gcs"
}

func (s *gcsObjectStore) PutObject(ctx context.Context, objectPath, contentType string, data []byte) error {
	cleanPath := strings.Trim(strings.TrimSpace(objectPath), "/")
	if cleanPath == "" {
		return errors.New("object path is required")
	}

	trimmedType := strings.TrimSpace(contentType)
	if trimmedType == "" {
		trimmedType = "application/octet-stream"
	}

	object := &gcsapi.Object{
		Name:        cleanPath,
		ContentType: trimmedType,
	}

	if _, err := s.service.Objects.Insert(s.bucketName, object).Media(bytes.NewReader(data)).Context(ctx).Do(); err != nil {
		return fmt.Errorf("write gcs object %q: %w", cleanPath, err)
	}
	return nil
}

func (s *gcsObjectStore) DeleteObject(ctx context.Context, objectPath string) error {
	cleanPath := strings.Trim(strings.TrimSpace(objectPath), "/")
	if cleanPath == "" {
		return nil
	}

	err := s.service.Objects.Delete(s.bucketName, cleanPath).Context(ctx).Do()
	if err == nil {
		return nil
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound {
		return nil
	}

	return fmt.Errorf("delete gcs object %q: %w", cleanPath, err)
}
