package handler

import (
	"time"
)

type BreakerConfig struct {
	ErrorThreshold   int
	SuccessThreshold int
	Timeout          time.Duration
}
