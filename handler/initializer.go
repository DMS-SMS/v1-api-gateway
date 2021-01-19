package handler

import (
	"log"
	"os"
)

var naverClientID string
var naverClientSecret string
var consulSecretHeader string

func init() {
	if naverClientID = os.Getenv("NAVER_CLIENT_ID"); naverClientID == "" {
		log.Fatal("please set NAVER_CLIENT_ID in environment variable")
	}
	if naverClientSecret = os.Getenv("NAVER_CLIENT_SECRET"); naverClientSecret == "" {
		log.Fatal("please set NAVER_CLIENT_SECRET in environment variable")
	}
	if consulSecretHeader = os.Getenv("CONSUL_SECRET_HEADER"); consulSecretHeader == "" {
		log.Fatal("please set CONSUL_SECRET_HEADER in environment variable")
	}
}

var limitTableForNaver = map[string]bool{}
