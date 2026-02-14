// models/s3.go
package models

type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	BaseURL         string
	Secure          bool
}
