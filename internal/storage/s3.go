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
	resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 list: %w", err)
	}
	var keys []string
	for _, obj := range resp.Contents {
		key := strings.TrimPrefix(*obj.Key, prefix)
		if key != "" {
			keys = append(keys, *obj.Key)
		}
	}
	return keys, nil
}
