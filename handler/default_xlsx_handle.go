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

	_ = reqID
	_ = topSpan

	adminRegex := regexp.MustCompile("^admin-\\d{12}$")
	if !adminRegex.MatchString(uuidClaims.UUID) {
		status, _code, msg := http.StatusForbidden, 0, "you are not admin"
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}

	if !strings.HasSuffix(receivedReq.Excel.Filename, ".xlsx") {
		status, _code := http.StatusBadRequest, code.IntegrityInvalidRequest
		msg := fmt.Sprintf("formatting of Excel file must be .xlsx, file name: %s", receivedReq.Excel.Filename)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}

	f, err := receivedReq.Excel.Open()
	if err != nil {
		status, _code := http.StatusBadRequest, code.IntegrityInvalidRequest
		msg := fmt.Sprintf("unable to open excel file, file name: %s, err: %v", receivedReq.Excel.Filename, err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	defer func() {
		_ = f.Close()
	}()

	excel, err := excelize.OpenReader(f)
	if err != nil {
		status, _code := http.StatusBadRequest, code.IntegrityInvalidRequest
		msg := fmt.Sprintf("unable to read excel file, file name: %s, err: %v", receivedReq.Excel.Filename, err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}

	status, _code, msg := 0, 0, ""
	defer func() {
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)})
		if status == http.StatusInternalServerError {
			entry.Error()
		} else {
			entry.Info()
		}
		return
	}()

	// 학년, 반, 번호, 이름, (+ 전화번호)
	for _, sheet := range excel.GetSheetMap() {
		rows, err := excel.GetRows(sheet)
		if err != nil {
			status = http.StatusBadRequest
			msg = err.Error()
			return
		}

		attrs := rows[0]
		var studentNumberIndex, nameIndex, phoneNumberIndex int
		var studentNumberExist, nameExist, phoneNumberExist bool

		// 속성 별 인덱스 조회
		for i, attr := range attrs {
			switch attr {
			case "학번", "student_number":
				studentNumberIndex = i
				studentNumberExist = true
			case "성명", "이름", "name":
				nameIndex = i
				nameExist = true
			case "전화번호", "전화 번호", "phone_number", "전화":
				phoneNumberIndex = i
				phoneNumberExist = true
			default:
				continue
			}
		}

		// 존재 X 속성 존재 시, 반환
		switch false {
		case studentNumberExist, nameExist, phoneNumberExist:
			status = http.StatusBadRequest
			msg = fmt.Sprintf("각각 학번, 성명, 전화번호와 관련된 속성이 모두 존재해야합니다, sheet: %s", sheet)
			return
		}

		// 최대 인덱스 값 구함
		var maxIndex int
		for i, e := range []int{studentNumberIndex, nameIndex, phoneNumberIndex} {
			if i==0 || e > maxIndex {
				maxIndex = e
			}
		}

		rowValues := rows[1:]
		for _, rowValue := range rowValues {
			if len(rowValue) - 1 < maxIndex {
				continue
			}

			studentNumber := rowValue[studentNumberIndex]
			name := rowValue[nameIndex]
			phoneNumber := rowValue[phoneNumberIndex]

			// 공백 값 -> continue
			switch true {
			case blankRegex.MatchString(studentNumber), blankRegex.MatchString(name), blankRegex.MatchString(phoneNumber):
				continue
			}

			// 띄워쓰기 삭제 (Ex, "박진홍 " -> "박진홍")
			studentNumber = strings.Join(strings.Split(studentNumber, " "), "")
			name = strings.Join(strings.Split(name, " "), "")
			phoneNumber = strings.Join(strings.Split(phoneNumber, " "), "")

			// 옳바르지 않은 형식의 데이터 존재 -> 400 반환
			switch false {
			case studentNumberRegex.MatchString(studentNumber), nameRegex.MatchString(name), phoneNumberRegex.MatchString(phoneNumber):
				status = http.StatusBadRequest
				msg = fmt.Sprintf("학번, 성명, 전화번호와 관련된 값이 옳바르지 않습니다. (%s %s %s) sheet: %s", studentNumber, name, phoneNumber, sheet)
				return
			}

			// 3109 박진홍 "01088378511"
			// fmt.Println(studentNumber, name, phoneNumber)
		}
	}
}
