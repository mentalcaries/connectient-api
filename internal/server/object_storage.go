package server

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type ObjectStorage interface {
	Put(context.Context, string, string, io.Reader, int64) error
	Delete(context.Context, string) error
	PublicURL(string) string
	OwnedKey(string, string) (string, bool)
}

type r2ObjectStorage struct {
	client     *s3.Client
	bucket     string
	publicBase string
	publicHost string
}

func newR2ObjectStorageFromEnv() ObjectStorage {
	accountID := os.Getenv("R2_ACCOUNT_ID")
	bucket := os.Getenv("R2_BUCKET_NAME")
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	publicBase := strings.TrimRight(os.Getenv("R2_PUBLIC_URL"), "/")
	if accountID == "" || bucket == "" || accessKey == "" || secretKey == "" || publicBase == "" {
		return nil
	}
	parsed, err := url.Parse(publicBase)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil
	}
	cfg := aws.Config{
		Region:       "auto",
		BaseEndpoint: aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) { options.UsePathStyle = true })
	return &r2ObjectStorage{client: client, bucket: bucket, publicBase: publicBase, publicHost: parsed.Host}
}

func (storage *r2ObjectStorage) Put(ctx context.Context, key, contentType string, body io.Reader, size int64) error {
	_, err := storage.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(storage.bucket), Key: aws.String(key), Body: body,
		ContentType: aws.String(contentType), ContentLength: aws.Int64(size),
	})
	return err
}

func (storage *r2ObjectStorage) Delete(ctx context.Context, key string) error {
	_, err := storage.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(storage.bucket), Key: aws.String(key),
	})
	return err
}

func (storage *r2ObjectStorage) PublicURL(key string) string {
	return storage.publicBase + "/" + key
}

func (storage *r2ObjectStorage) OwnedKey(rawURL, practiceID string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != storage.publicHost {
		return "", false
	}
	key := strings.TrimPrefix(parsed.EscapedPath(), "/")
	decoded, err := url.PathUnescape(key)
	if err != nil || !strings.HasPrefix(decoded, practiceID+"/") || strings.Contains(decoded, "..") {
		return "", false
	}
	return decoded, true
}
