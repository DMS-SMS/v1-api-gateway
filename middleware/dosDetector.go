package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type dosDetector struct {
	limitTable map[string]*uint32
	rejected   map[string]bool
	mutex      sync.Mutex
}

func DosDetector() gin.HandlerFunc {
	return (&dosDetector{
		limitTable: make(map[string]*uint32),
		rejected:   make(map[string]bool),
		mutex:      sync.Mutex{},
	}).detectDos
}

func (d *dosDetector) detectDos (c *gin.Context) {
	cip := c.ClientIP()

	d.mutex.Lock()
	defer d.mutex.Unlock()

	// set initial value if not exists
	if _, ok := d.limitTable[cip]; !ok {
		var init uint32 = 0
		d.limitTable[cip] = &init
	}
	if _, ok := d.rejected[cip]; !ok {
		d.rejected[cip] = false
	}

	// check ip is blocked
	if d.rejected[cip] {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		}{
			Status: http.StatusTooManyRequests,
			Message: "your IP is currently request blocked",
		})
		return
	}

	// set rejected true if total request per second is over than 10
	if *d.limitTable[cip] >= 10 {
		d.rejected[cip] = true
		time.AfterFunc(time.Minute, func() {
			d.mutex.Lock()
			d.rejected[cip] = false
			d.mutex.Unlock()
		})
		c.AbortWithStatusJSON(http.StatusTooManyRequests, struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		}{
			Status:  http.StatusTooManyRequests,
			Message: "unusual request was detected. Please try again after a minute",
		})
		return
	}

	atomic.AddUint32(d.limitTable[cip], 1)
	time.AfterFunc(time.Second, func() {
		d.mutex.Lock()
		*d.limitTable[cip] -= 1
		d.mutex.Unlock()
	})
}
