package handler

import (
	"log"
	"os"
)

var naverClientID string
var naverClientSecret string

func init() {
	if naverClientID = os.Getenv("NAVER_CLIENT_ID"); naverClientID == "" {
		log.Fatal("please set NAVER_CLIENT_ID in environment variable")
	}
	if naverClientSecret = os.Getenv("NAVER_CLIENT_SECRET"); naverClientSecret == "" {
		log.Fatal("please set NAVER_CLIENT_SECRET in environment variable")
	}
}
