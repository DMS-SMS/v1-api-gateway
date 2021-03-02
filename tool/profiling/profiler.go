// add package in v.1.0.5
// profiling package is used for profiling cpu, memory, block & upload file to s3
// profiler.go is file that profile & uploading every few days in init function

package profiling

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	log "github.com/micro/go-micro/v2/logger"
	systemlog "log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

func init() {
	cmdFinSig := make(chan os.Signal, 1)
	signal.Notify(cmdFinSig, syscall.SIGINT, syscall.SIGTERM)

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
				systemlog.Fatal(err)
			}
			cpuProfFile, err := os.Create(cpuProf)
			if err != nil {
				systemlog.Fatal(err)
			}
			memoryProfFile, err := os.Create(memoryProf)
			if err != nil {
				systemlog.Fatal(err)
			}
			blockProfFile, err := os.Create(blockProf)
			if err != nil {
				systemlog.Fatal(err)
			}

			// Ex) profiles/gateway/v.1.0.5/2021-03-02/13:40:31/cpu.prof
			profS3Path := fmt.Sprintf("profiling/gateway/v.%s/%s/%s", version, nowDate, nowTime)
			cpuProfS3 := profS3Path + "/cpu.prof"
			memoryProfS3 := profS3Path + "/memory.prof"
			blockProfS3 := profS3Path + "/block.prof"

			log.Info("start profiling cpu, memory, block")
			if err := pprof.StartCPUProfile(cpuProfFile); err != nil {
				systemlog.Fatal(err)
			}

			// 시작 당일이 끝날 때 까지 대기
			afterOneDay := now.AddDate(0, 0, 1)
			tomorrow := time.Date(afterOneDay.Year(), afterOneDay.Month(), afterOneDay.Day(), 0, 0, 0, 0, time.UTC)
			timeFinSig := time.Tick(tomorrow.Sub(now))

			select {
			case <-timeFinSig:
				log.Info("upload profiling result recorded on this day")
			case <-cmdFinSig:
				log.Info("upload profiling result as shutdown of process")
			}

			runtime.GC()
			_ = pprof.Lookup("heap").WriteTo(memoryProfFile, 1)
			_ = pprof.Lookup("block").WriteTo(blockProfFile, 1)
			pprof.StopCPUProfile()

			_ = cpuProfFile.Close()
			_ = memoryProfFile.Close()
			_ = blockProfFile.Close()

			cpuProfFile, _ = os.Open(cpuProf)
			memoryProfFile, _ = os.Open(memoryProf)
			blockProfFile, _ = os.Open(blockProf)

			svc := s3.New(globalSession)
			if _, err := svc.PutObject(&s3.PutObjectInput{
				Body:   cpuProfFile,
				Bucket: aws.String(s3Bucket),
				Key:    aws.String(cpuProfS3),
			}); err != nil {
				systemlog.Fatalf("unable to upload cpu profiling result to s3, err: %v", err)
			}
			if _, err := svc.PutObject(&s3.PutObjectInput{
				Body:   memoryProfFile,
				Bucket: aws.String(s3Bucket),
				Key:    aws.String(memoryProfS3),
			}); err != nil {
				systemlog.Fatalf("unable to upload memory profiling result to s3, err: %v", err)
			}
			if _, err := svc.PutObject(&s3.PutObjectInput{
				Body:   blockProfFile,
				Bucket: aws.String(s3Bucket),
				Key:    aws.String(blockProfS3),
			}); err != nil {
				systemlog.Fatalf("unable to upload block profiling result to s3, err: %v", err)
			}
		}
	}()
}
