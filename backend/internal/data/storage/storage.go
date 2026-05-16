// Package storage wraps the AWS S3 SDK so the rest of the backend can put,
// fetch, and sign URLs for objects without caring whether the bucket lives in
// S3 or an S3-compatible service. Uploads stream through manager.Uploader so
// the backend never has to buffer a whole file in memory — important for the
// 25MB attachment cap × concurrent uploads.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// FileStore is the abstract surface handlers depend on, so tests can swap in
// an in-memory fake.
type FileStore interface {
	UploadStream(ctx context.Context, key string, body io.Reader, contentType string) (int64, error)
	Delete(ctx context.Context, key string) error
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	ListWithPrefix(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// ObjectInfo is the minimal metadata the orphan sweep needs: the key and the
// last-modified timestamp so we can honour the grace window.
type ObjectInfo struct {
	Key          string
	LastModified time.Time
}

// Config bundles everything InitStorage needs. All fields are required except
// Region (defaults to eu-west-1) and UsePathStyle (defaults to true, required
// for MinIO).
type Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	UsePathStyle    bool
}

// Storage is the concrete FileStore implementation backed by S3-compatible
// object storage.
type Storage struct {
	client   *s3.Client
	uploader *manager.Uploader
	presign  *s3.PresignClient
	bucket   string
}

// InitStorage builds the S3 client, ensures the bucket exists, and returns a
// FileStore-ready Storage. Returns an error if any required credential is
// missing or the bucket can't be reached/created.
func InitStorage(cfg Config) (*Storage, error) {
	if cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" || cfg.Bucket == "" {
		return nil, errors.New("storage: endpoint, access key, secret key, and bucket are all required")
	}
	if cfg.Region == "" {
		cfg.Region = "eu-west-1"
	}
	endpoint := cfg.Endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})

	if err := ensureBucket(client, cfg.Bucket); err != nil {
		return nil, err
	}

	return &Storage{
		client:   client,
		uploader: manager.NewUploader(client),
		presign:  s3.NewPresignClient(client),
		bucket:   cfg.Bucket,
	}, nil
}

// ensureBucket performs HeadBucket + CreateBucket if missing.
func ensureBucket(client *s3.Client, bucket string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		log.Printf("storage: bucket %q ready", bucket)
		return nil
	}

	log.Printf("storage: bucket %q missing (%v); attempting to create", bucket, err)
	_, createErr := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if createErr != nil {
		var alreadyOwned *types.BucketAlreadyOwnedByYou
		if errors.As(createErr, &alreadyOwned) {
			return nil
		}
		return fmt.Errorf("storage: create bucket %q: %w", bucket, createErr)
	}
	log.Printf("storage: bucket %q created", bucket)
	return nil
}

// countingReader wraps an io.Reader to track the total bytes that passed
// through it — needed because streaming uploads don't know their size up front
// but we want to record it in the attachments row.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// UploadStream pushes the reader's bytes to S3 under the given key, using
// multipart upload internally for unknown-length streams. Returns the total
// number of bytes written.
func (s *Storage) UploadStream(ctx context.Context, key string, body io.Reader, contentType string) (int64, error) {
	cr := &countingReader{r: body}
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        cr,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return 0, fmt.Errorf("storage: upload %q: %w", key, err)
	}
	return cr.n, nil
}

// Delete removes the object at key. Returns nil even if the object doesn't
// exist — S3 DELETE is idempotent.
func (s *Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}

// SignedURL returns a time-limited GET URL for the object. ttl is how long the
// URL stays valid.
//
// The URL forces Content-Disposition: attachment so a top-level open of an
// uploaded HTML/SVG downloads instead of executing in the storage origin
// (stored-XSS defence). It does not affect <img> previews or the download
// link — Content-Disposition is ignored for embedded subresources.
func (s *Storage) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String("attachment"),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage: presign %q: %w", key, err)
	}
	return req.URL, nil
}

// ListWithPrefix returns metadata for every object under the given prefix.
// Used by the orphan-sweep job: pairs each key with its last-modified time so
// the sweep can honour a grace window for in-flight uploads.
func (s *Storage) ListWithPrefix(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage: list %q: %w", prefix, err)
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			info := ObjectInfo{Key: *obj.Key}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			out = append(out, info)
		}
	}
	return out, nil
}
