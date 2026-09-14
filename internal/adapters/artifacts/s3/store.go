package s3store

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/iiwish/semlia/internal/adapters/artifacts/local"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

type Client interface {
	HeadBucket(context.Context, *awss3.HeadBucketInput, ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error)
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	DeleteObject(context.Context, *awss3.DeleteObjectInput, ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
}

type Options struct {
	Bucket         string
	Region         string
	Endpoint       string
	RequireHTTPS   bool
	SpoolDirectory string
}

type Store struct {
	client         Client
	bucket         string
	spoolDirectory string
}

func New(ctx context.Context, options Options) (*Store, error) {
	options.Bucket, options.Region, options.Endpoint = strings.TrimSpace(options.Bucket), strings.TrimSpace(options.Region), strings.TrimSpace(options.Endpoint)
	if options.Bucket == "" && options.Region == "" && options.Endpoint == "" {
		return &Store{}, nil
	}
	if options.Bucket == "" || options.Region == "" {
		return nil, ingestion.ErrStore
	}
	if options.Endpoint != "" {
		parsed, err := url.Parse(options.Endpoint)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Path != "" && parsed.Path != "/") || (options.RequireHTTPS && parsed.Scheme != "https") ||
			(parsed.Scheme != "https" && parsed.Scheme != "http") {
			return nil, ingestion.ErrStore
		}
	}
	configuration, err := config.LoadDefaultConfig(ctx, config.WithRegion(options.Region))
	if err != nil {
		return nil, ingestion.ErrStore
	}
	client := awss3.NewFromConfig(configuration, func(value *awss3.Options) {
		if options.Endpoint != "" {
			value.BaseEndpoint = aws.String(options.Endpoint)
			value.UsePathStyle = true
		}
	})
	store := NewWithClient(client, options.Bucket)
	store.spoolDirectory = strings.TrimSpace(options.SpoolDirectory)
	return store, nil
}

func NewWithClient(client Client, bucket string) *Store {
	return &Store{client: client, bucket: strings.TrimSpace(bucket)}
}

func (store *Store) Configured() bool {
	return store != nil && store.client != nil && store.bucket != ""
}

func (store *Store) Check(ctx context.Context) error {
	if !store.Configured() {
		return ingestion.ErrStore
	}
	if _, err := store.client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(store.bucket)}); err != nil {
		return ingestion.ErrStore
	}
	return nil
}

func (store *Store) Put(ctx context.Context, workspace identity.WorkspaceID, digest string, content io.Reader, size int64) error {
	if !store.Configured() || workspace.IsZero() || content == nil || size < 1 || size > ingestion.MaxUploadBytes {
		return ingestion.ErrStore
	}
	digestBytes, err := decodeDigest(digest)
	if err != nil {
		return ingestion.ErrStore
	}
	key := local.StorageKey(workspace, digest)
	if key == "" {
		return ingestion.ErrStore
	}
	file, temporaryName, written, actualDigest, err := spool(ctx, content, ingestion.MaxUploadBytes, store.spoolDirectory)
	if err != nil {
		return ingestion.ErrStore
	}
	defer func() { _ = file.Close(); _ = os.Remove(temporaryName) }()
	if written != size || actualDigest != digest {
		return ingestion.ErrStore
	}
	_, err = store.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(store.bucket), Key: aws.String(key), Body: file,
		ContentLength: aws.Int64(size), ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(digestBytes)),
		IfNoneMatch: aws.String("*"),
	})
	if err == nil {
		return nil
	}
	reader, openErr := store.Open(ctx, workspace, digest)
	if openErr != nil {
		return ingestion.ErrStore
	}
	defer reader.Close()
	verified, readErr := io.ReadAll(io.LimitReader(reader, size+1))
	if readErr != nil || int64(len(verified)) != size {
		return ingestion.ErrStore
	}
	return nil
}

func (store *Store) Open(ctx context.Context, workspace identity.WorkspaceID, digest string) (io.ReadCloser, error) {
	if !store.Configured() || workspace.IsZero() {
		return nil, ingestion.ErrStore
	}
	if _, err := decodeDigest(digest); err != nil {
		return nil, ingestion.ErrStore
	}
	key := local.StorageKey(workspace, digest)
	result, err := store.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(key)})
	if err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) && (apiError.ErrorCode() == "NoSuchKey" || apiError.ErrorCode() == "NotFound") {
			return nil, ingestion.ErrNotFound
		}
		return nil, ingestion.ErrStore
	}
	if result == nil || result.Body == nil {
		return nil, ingestion.ErrStore
	}
	defer result.Body.Close()
	file, temporaryName, written, actualDigest, err := spool(ctx, result.Body, ingestion.MaxUploadBytes, store.spoolDirectory)
	if err != nil || written < 1 || actualDigest != digest {
		if file != nil {
			_ = file.Close()
		}
		if temporaryName != "" {
			_ = os.Remove(temporaryName)
		}
		return nil, ingestion.ErrStore
	}
	return &temporaryReader{File: file, name: temporaryName}, nil
}

type temporaryReader struct {
	*os.File
	name string
}

func (reader *temporaryReader) Close() error {
	err := reader.File.Close()
	removeErr := os.Remove(reader.name)
	if err != nil {
		return err
	}
	return removeErr
}

func spool(ctx context.Context, source io.Reader, limit int64, directory string) (*os.File, string, int64, string, error) {
	file, err := os.CreateTemp(directory, "semlia-artifact-*")
	if err != nil {
		return nil, "", 0, "", err
	}
	name := file.Name()
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var total int64
	cleanup := func() { _ = file.Close(); _ = os.Remove(name) }
	for {
		if err := ctx.Err(); err != nil {
			cleanup()
			return nil, "", total, "", err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			total += int64(read)
			if total > limit {
				cleanup()
				return nil, "", total, "", ingestion.ErrLimitExceeded
			}
			if _, err := file.Write(buffer[:read]); err != nil {
				cleanup()
				return nil, "", total, "", err
			}
			_, _ = hash.Write(buffer[:read])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			cleanup()
			return nil, "", total, "", readErr
		}
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return nil, "", total, "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, "", total, "", err
	}
	return file, name, total, "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (store *Store) Delete(ctx context.Context, workspace identity.WorkspaceID, digest string) error {
	if !store.Configured() || workspace.IsZero() {
		return ingestion.ErrStore
	}
	if _, err := decodeDigest(digest); err != nil {
		return ingestion.ErrStore
	}
	key := local.StorageKey(workspace, digest)
	if _, err := store.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(key)}); err != nil {
		return ingestion.ErrStore
	}
	return nil
}

func decodeDigest(value string) ([]byte, error) {
	if !strings.HasPrefix(value, "sha256:") {
		return nil, errors.New("invalid digest")
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	if err != nil || len(decoded) != sha256.Size {
		return nil, errors.New("invalid digest")
	}
	return decoded, nil
}
