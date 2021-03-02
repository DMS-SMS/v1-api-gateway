package profiling

import (
	"fmt"
	"time"
)

func init() {
	go func() {
		for {
			now := time.Now()
			if now.Location().String() == time.UTC.String() {
				now = now.Add(time.Hour * 9)
			}
			nowDate := fmt.Sprintf("%4d-%02d-%02d", now.Year(), now.Month(), now.Day())
			nowTime := fmt.Sprintf("%02d:%02d:%02d", now.Hour(), now.Minute(), now.Second())

		}
	}()
}
