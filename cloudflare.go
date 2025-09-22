package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type CloudflareS3 struct {
	client *s3.Client
	bucket string
}

func NewCloudflareS3() (*CloudflareS3, error) {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: os.Getenv("CLOUDFLARE_ENDPOINT"),
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(r2Resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			os.Getenv("CLOUDFLARE_ACCESS_KEY"),
			os.Getenv("CLOUDFLARE_SECRET_KEY"),
			"",
		)),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	return &CloudflareS3{
		client: client,
		bucket: "recipes",
	}, nil
}

func (c *CloudflareS3) UploadImage(filename, contentType string, content []byte) error {
	_, err := c.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(content),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to upload image: %w", err)
	}
	return nil
}
