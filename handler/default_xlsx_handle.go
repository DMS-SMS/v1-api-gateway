// add file in v.1.0.5
// default_xlsx_handle.go is file that declare xlsx handling method

package handler

import (
	"encoding/json"
	"fmt"
	"gateway/entity"
	jwtutil "gateway/tool/jwt"
	code "gateway/utils/code/golang"
	"github.com/360EntSecGroup-Skylar/excelize/v2"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"net/http"
	"regexp"
	"strings"
)

var (
	studentNumberRegex = regexp.MustCompile("(^[1-3][1-4])([0-1][0-9]$|20|21)")
	nameRegex = regexp.MustCompile("^[가-힣]+$")
	phoneNumberRegex = regexp.MustCompile("^\"\\d{2,3}[-_.]?\\d{3,4}[-_.]?\\d{4}\"$")
	blankRegex = regexp.MustCompile("(^[ ]+$)|(^$)")
)

func (h *_default) AddUnsignedStudentsFromExcel(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// get token claim from middleware
	inAdvanceClaims, _ := c.Get("Claims")
	uuidClaims, _ := inAdvanceClaims.(jwtutil.UUIDClaims)
	entry = entry.WithField("user_uuid", uuidClaims.UUID)

	// get bound request entry from middleware
	inAdvanceReq, _ := c.Get("Request")
	receivedReq, _ := inAdvanceReq.(*entity.AddUnsignedStudentsFromExcelRequest)
	reqBytes, _ := json.Marshal(receivedReq)
}
