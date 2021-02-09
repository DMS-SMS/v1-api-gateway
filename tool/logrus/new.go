// add package in v.1.0.4
// this package is used to separate generating logrus.Logger from main

package logrus

import (
	"io"
)

type noneWriter struct {
	io.Writer
}

func (n noneWriter) Write(p []byte) (_ int, _ error) {
	return
}
