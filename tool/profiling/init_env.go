package profiling

import (
"log"
"os"
)

var s3Bucket string
var version string

func init() {
	if s3Bucket = os.Getenv("SMS_AWS_BUCKET"); s3Bucket == "" {
		log.Fatal("please set SMS_AWS_BUCKET in environment variable")
	}
	if version = os.Getenv("VERSION"); s3Bucket == "" {
		log.Fatal("please set VERSION in environment variable")
}

}
