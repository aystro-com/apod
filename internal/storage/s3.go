package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	client *s3.Client
	bucket string
}

func NewS3(config map[string]string) (*S3Storage, error) {
	bucket := config["bucket"]
	if bucket == "" {
		return nil, fmt.Errorf("s3: bucket is required")
	}

	region := config["region"]
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(config["access_key"], config["secret_key"], ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("s3: load config: %w", err)
	}

	var opts []func(*s3.Options)
	if endpoint := config["endpoint"]; endpoint != "" {
		// A custom endpoint receives the credentials and the backup stream.
		// Require https so they aren't sent in cleartext; an explicit opt-in
		// allows http for trusted internal/dev endpoints (e.g. MinIO).
		allowInsecure := strings.EqualFold(strings.TrimSpace(config["insecure_endpoint"]), "true")
		if err := validateEndpointURL(endpoint, allowInsecure); err != nil {
			return nil, fmt.Errorf("s3: %w", err)
		}
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(cfg, opts...)
	return &S3Storage{client: client, bucket: bucket}, nil
}

func (s *S3Storage) Upload(ctx context.Context, key string, reader io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("s3 upload: %w", err)
	}
	return nil
}

func (s *S3Storage) Download(ctx context.Context, key string, writer io.Writer) error {
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("s3 download: %w", err)
	}
	defer resp.Body.Close()
	_, err = io.Copy(writer, resp.Body)
	return err
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	return nil
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	var token *string
	for {
		resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            &s.bucket,
			Prefix:            &prefix,
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("s3 list: %w", err)
		}
		for _, obj := range resp.Contents {
			// Return the full key (prefix included), matching the local and SFTP
			// drivers, which build keys as filepath.Join(prefix, name). Skip the
			// prefix "directory" placeholder itself.
			if strings.TrimPrefix(*obj.Key, prefix) != "" {
				keys = append(keys, *obj.Key)
			}
		}
		// Page through: ListObjectsV2 caps a single response at 1000 objects.
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
		token = resp.NextContinuationToken
	}
	return keys, nil
}
