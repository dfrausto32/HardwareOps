package objectstore

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStore struct {
	client *minio.Client
	// objectLock enables WORM retention on buckets this store creates
	// (ransomware control R-01): objects cannot be deleted or overwritten
	// until the retention period expires, even by the application credential.
	objectLock        bool
	lockRetentionDays int

	lockMu         sync.Mutex
	lockConfigured map[string]bool
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Region    string
	// ObjectLock creates buckets with object locking and applies a default
	// GOVERNANCE retention of ObjectLockRetentionDays. Locking can only be
	// enabled at bucket creation: pointing this at a pre-existing unlocked
	// bucket fails EnsureBucket with a migration hint.
	ObjectLock              bool
	ObjectLockRetentionDays int
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
	days := cfg.ObjectLockRetentionDays
	if days <= 0 {
		days = 35
	}
	return &MinIOStore{
		client:            client,
		objectLock:        cfg.ObjectLock,
		lockRetentionDays: days,
		lockConfigured:    map[string]bool{},
	}, nil
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
	if !ok {
		if err := s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{ObjectLocking: s.objectLock}); err != nil {
			return err
		}
	}
	if s.objectLock {
		return s.ensureObjectLockConfig(ctx, bucket)
	}
	return nil
}

// ensureObjectLockConfig applies the default GOVERNANCE retention rule. It
// fails for buckets created without object locking — locking cannot be
// retrofitted, so the operator must migrate objects to a new locked bucket.
func (s *MinIOStore) ensureObjectLockConfig(ctx context.Context, bucket string) error {
	s.lockMu.Lock()
	done := s.lockConfigured[bucket]
	s.lockMu.Unlock()
	if done {
		return nil
	}
	mode := minio.Governance
	validity := uint(s.lockRetentionDays)
	unit := minio.Days
	if err := s.client.SetObjectLockConfig(ctx, bucket, &mode, &validity, &unit); err != nil {
		return fmt.Errorf("object lock requested (S3_OBJECT_LOCK=1) but bucket %q does not support it — object locking can only be enabled at bucket creation; migrate objects to a freshly created locked bucket: %w", bucket, err)
	}
	s.lockMu.Lock()
	s.lockConfigured[bucket] = true
	s.lockMu.Unlock()
	return nil
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
