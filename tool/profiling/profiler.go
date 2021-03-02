package profiling

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
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

			// Ex) /usr/share/gateway/profile/v.1.0.5/2021-03-02/13:40:31/cpu.prof
			profPath := fmt.Sprintf("/usr/share/gateway/profile/v.%s/%s/%s", version, nowDate, nowTime)
			cpuProf := profPath + "/cpu.prof"
			memoryProf := profPath + "/memory.prof"
			blockProf := profPath + "/block.prof"

			if err := os.MkdirAll(profPath, os.ModePerm); err != nil {
				log.Fatal(err)
			}
			cpuProfFile, err := os.Create(cpuProf)
			if err != nil {
				log.Fatal(err)
			}
			memoryProfFile, err := os.Create(memoryProf)
			if err != nil {
				log.Fatal(err)
			}
			blockProfFile, err := os.Create(blockProf)
			if err != nil {
				log.Fatal(err)
			}

			// Ex) profiles/gateway/v.1.0.5/2021-03-02/13:40:31/cpu.prof
			profS3Path := fmt.Sprintf("profiling/gateway/v.%s/%s/%s", version, nowDate, nowTime)
			cpuProfS3 := profS3Path + "/cpu.prof"
			memoryProfS3 := profS3Path + "/memory.prof"
			blockProfS3 := profS3Path + "/block.prof"

			fmt.Println("start profiling cpu, memory, block")
			if err := pprof.StartCPUProfile(cpuProfFile); err != nil {
				log.Fatal(err)
			}

			// 시작 당일이 끝날 때 까지 대기
			afterOneDay := now.AddDate(0, 0, 1)
			tomorrow := time.Date(afterOneDay.Year(), afterOneDay.Month(), afterOneDay.Day(), 0, 0, 0, 0, time.UTC)
			time.Sleep(tomorrow.Sub(now))

			fmt.Println("Uploading profile result to S3")
			runtime.GC()
			_ = pprof.Lookup("heap").WriteTo(memoryProfFile, 1)
			_ = pprof.Lookup("block").WriteTo(blockProfFile, 1)
			pprof.StopCPUProfile()

			_ = cpuProfFile.Close()
			_ = memoryProfFile.Close()
			_ = blockProfFile.Close()
		}
	}()
}
