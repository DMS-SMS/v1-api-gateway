// add package in v.1.0.4
// this package is used to separate generating logrus.Logger from main

package logrus

import (
	logrustash "github.com/bshuster-repo/logrus-logstash-hook"
	"github.com/sirupsen/logrus"
	"io"
	"log"
	"os"
)

type noneWriter struct {
	io.Writer
}

func (n noneWriter) Write(p []byte) (_ int, _ error) {
	return
}

func New(filepath string, fields logrus.Fields) (logger *logrus.Logger) {
	logfile, err := os.OpenFile(filepath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil {
		log.Fatalf("unable to open file, err: %v\n", err)
		return
	}

	logger = logrus.New()
	logger.SetOutput(noneWriter{})
	logger.Hooks.Add(logrustash.New(logfile, logrustash.DefaultFormatter(fields)))
	return
}
