package objectstore

import (
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStore struct {
	client *minio.Client
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Region    string
}

func NewMinIO(cfg MinIOConfig) (*MinIOStore, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  minioCredentials(cfg),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	return &MinIOStore{client: client}, nil
}

func (s *MinIOStore) PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error) {
	url, err := s.client.PresignedGetObject(ctx, bucket, key, expires, nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func (s *MinIOStore) PresignPut(ctx context.Context, bucket, key string, expires time.Duration, contentType string) (string, error) {
	_ = contentType
	url, err := s.client.PresignedPutObject(ctx, bucket, key, expires)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func (s *MinIOStore) PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error) {
	info, err := s.client.PutObject(ctx, bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (s *MinIOStore) EnsureBucket(ctx context.Context, bucket string) error {
	ok, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
}

func (s *MinIOStore) DeleteObject(ctx context.Context, bucket, key string) error {
	return s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

func (s *MinIOStore) StatObject(ctx context.Context, bucket, key string) (int64, error) {
	info, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (s *MinIOStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func minioCredentials(cfg MinIOConfig) *credentials.Credentials {
	if minioCredentialMode(cfg) == "static" {
		return credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	}
	return credentials.NewIAM("")
}

func minioCredentialMode(cfg MinIOConfig) string {
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		return "static"
	}
	return "iam"
}
