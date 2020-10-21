package middleware

import "sync"

type dosDetector struct {
	limitTable map[string]*uint32
	rejected   map[string]bool
	mutex      sync.Mutex
}

func DosDetector() *dosDetector {
	return &dosDetector{
		limitTable: make(map[string]*uint32),
		rejected:   make(map[string]bool),
		mutex:      sync.Mutex{},
	}
}
