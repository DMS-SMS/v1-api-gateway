package profiling

import (
	"gateway/tool/env"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
"os"
)

var s3Bucket string
var version string
var globalSession *session.Session

func init() {
	if s3Bucket = os.Getenv("SMS_AWS_BUCKET"); s3Bucket == "" {
		log.Fatal("please set SMS_AWS_BUCKET in environment variable")
	}
	if version = os.Getenv("VERSION"); s3Bucket == "" {
		log.Fatal("please set VERSION in environment variable")
	}

	awsId := env.GetAndFatalIfNotExits("SMS_AWS_ID")
	awsKey := env.GetAndFatalIfNotExits("SMS_AWS_KEY")
	s3Region := env.GetAndFatalIfNotExits("SMS_AWS_REGION")
	if awsSession, err := session.NewSession(&aws.Config{
		Region:      aws.String(s3Region),
		Credentials: credentials.NewStaticCredentials(awsId, awsKey, ""),
	}); err != nil {
		log.Fatalf("unable to create aws session, err: %v", err)
	} else {
		globalSession = awsSession
	}
}
