package s3store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

type fakeClient struct {
	head func(context.Context, *awss3.HeadBucketInput) (*awss3.HeadBucketOutput, error)
	put  func(context.Context, *awss3.PutObjectInput) (*awss3.PutObjectOutput, error)
	get  func(context.Context, *awss3.GetObjectInput) (*awss3.GetObjectOutput, error)
	del  func(context.Context, *awss3.DeleteObjectInput) (*awss3.DeleteObjectOutput, error)
}

func (client fakeClient) HeadBucket(ctx context.Context, input *awss3.HeadBucketInput, _ ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error) {
	if client.head == nil {
		return &awss3.HeadBucketOutput{}, nil
	}
	return client.head(ctx, input)
}

func (client fakeClient) PutObject(ctx context.Context, input *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	return client.put(ctx, input)
}
func (client fakeClient) GetObject(ctx context.Context, input *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	return client.get(ctx, input)
}
func (client fakeClient) DeleteObject(ctx context.Context, input *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	return client.del(ctx, input)
}

func TestPutVerifiesBodyAndPreconditionReplay(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	content := []byte("immutable")
	digest := digestOf(content)
	client := fakeClient{
		put: func(_ context.Context, input *awss3.PutObjectInput) (*awss3.PutObjectOutput, error) {
			if aws.ToString(input.IfNoneMatch) != "*" || input.ContentLength == nil || *input.ContentLength != int64(len(content)) {
				t.Fatal("missing conditional immutable put")
			}
			stored, _ := io.ReadAll(input.Body)
			if !bytes.Equal(stored, content) {
				t.Fatal("wrong put body")
			}
			return nil, &smithy.GenericAPIError{Code: "PreconditionFailed", Message: "exists"}
		},
		get: func(context.Context, *awss3.GetObjectInput) (*awss3.GetObjectOutput, error) {
			return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(content))}, nil
		},
		del: successfulDelete,
	}
	store := NewWithClient(client, "bucket")
	if err := store.Put(context.Background(), workspace, digest, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), workspace, digest, bytes.NewReader(content), int64(len(content)+1)); !errors.Is(err, ingestion.ErrStore) {
		t.Fatalf("size mismatch = %v", err)
	}
}

func TestOpenClassifiesNotFoundAndStoreFailuresAndRejectsCorruption(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	digest := digestOf([]byte("expected"))
	for _, test := range []struct {
		name string
		get  func(context.Context, *awss3.GetObjectInput) (*awss3.GetObjectOutput, error)
		want error
	}{
		{"not-found", func(context.Context, *awss3.GetObjectInput) (*awss3.GetObjectOutput, error) {
			return nil, &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}
		}, ingestion.ErrNotFound},
		{"forbidden", func(context.Context, *awss3.GetObjectInput) (*awss3.GetObjectOutput, error) {
			return nil, &smithy.GenericAPIError{Code: "AccessDenied", Message: "no"}
		}, ingestion.ErrStore},
		{"outage", func(context.Context, *awss3.GetObjectInput) (*awss3.GetObjectOutput, error) {
			return nil, errors.New("network unavailable")
		}, ingestion.ErrStore},
		{"corrupt", func(context.Context, *awss3.GetObjectInput) (*awss3.GetObjectOutput, error) {
			return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte("corrupt!")))}, nil
		}, ingestion.ErrStore},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewWithClient(fakeClient{put: successfulPut, get: test.get, del: successfulDelete}, "bucket")
			if _, err := store.Open(context.Background(), workspace, digest); !errors.Is(err, test.want) {
				t.Fatalf("Open() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDeletePropagatesStoreFailure(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	store := NewWithClient(fakeClient{put: successfulPut, get: nil, del: func(context.Context, *awss3.DeleteObjectInput) (*awss3.DeleteObjectOutput, error) {
		return nil, errors.New("outage")
	}}, "bucket")
	if err := store.Delete(context.Background(), workspace, digestOf([]byte("x"))); !errors.Is(err, ingestion.ErrStore) {
		t.Fatalf("Delete() = %v", err)
	}
}

func TestCheckRequiresReachableBucket(t *testing.T) {
	store := NewWithClient(fakeClient{head: func(context.Context, *awss3.HeadBucketInput) (*awss3.HeadBucketOutput, error) {
		return nil, errors.New("unreachable")
	}}, "bucket")
	if err := store.Check(context.Background()); !errors.Is(err, ingestion.ErrStore) {
		t.Fatalf("Check() = %v", err)
	}
}

func successfulPut(context.Context, *awss3.PutObjectInput) (*awss3.PutObjectOutput, error) {
	return &awss3.PutObjectOutput{}, nil
}
func successfulDelete(context.Context, *awss3.DeleteObjectInput) (*awss3.DeleteObjectOutput, error) {
	return &awss3.DeleteObjectOutput{}, nil
}
func digestOf(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}
