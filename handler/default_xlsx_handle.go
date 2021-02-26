// add file in v.1.0.5
// default_xlsx_handle.go is file that declare xlsx handling method

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"gateway/entity"
	authproto "gateway/proto/golang/auth"
	jwtutil "gateway/tool/jwt"
	code "gateway/utils/code/golang"
	topic "gateway/utils/topic/golang"
	"github.com/360EntSecGroup-Skylar/excelize/v2"
	"github.com/gin-gonic/gin"
	"github.com/micro/go-micro/v2/client"
	"github.com/micro/go-micro/v2/errors"
	"github.com/micro/go-micro/v2/metadata"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/sirupsen/logrus"
	"github.com/uber/jaeger-client-go"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var (
	studentNumberRegex = regexp.MustCompile("(^[1-3][1-4])([0-1][0-9]$|20|21)")
	nameRegex = regexp.MustCompile("^[가-힣]{2,4}$")
	phoneNumberRegex = regexp.MustCompile("^\"\\d{3}[-_.]?\\d{4}[-_.]?\\d{4}\"$")
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

	sheets := excel.GetSheetMap()
	if sheet := c.Param("sheet"); sheet != "" {
		sheets = map[int]string{1: sheet}
	}

	// 학년, 반, 번호, 이름, (+ 전화번호)
	type student struct {
		name, phoneNumber     string
		grade, _class, number int
	}

	// 엑셀 파일 파싱 (힉생 정보 조회)
	var students []student
	for _, sheet := range sheets {
		rows, err := excel.GetRows(sheet)
		if err != nil {
			status, _code := http.StatusBadRequest, 0
			msg := err.Error()
			c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
			return
		}

		// 속성 행, 필드 행이 존재하지 않다면 continue
		if len(rows) <= 2 || rows[0] == nil {
			continue
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
			status, _code := http.StatusBadRequest, 0
			msg := fmt.Sprintf("각각 학번, 성명, 전화번호와 관련된 속성이 모두 존재해야합니다, sheet: %s", sheet)
			c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
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
			if rowValue == nil || len(rowValue) - 1 < maxIndex {
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
				status, _code := http.StatusBadRequest, 0
				msg := fmt.Sprintf("학번, 성명, 전화번호와 관련된 값이 옳바르지 않습니다. (%s %s %s) sheet: %s", studentNumber, name, phoneNumber, sheet)
				c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
				entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
				return
			}

			phoneNumber = regexp.MustCompile("[-_.]").ReplaceAllString(phoneNumber, "")
			grade, _ := strconv.Atoi(studentNumber[:1])
			_class, _ := strconv.Atoi(studentNumber[1:2])
			number, _ := strconv.Atoi(studentNumber[2:])

			students = append(students, student{
				name:        name,
				phoneNumber: strings.TrimPrefix(strings.TrimSuffix(phoneNumber, "\""), "\""),
				grade:       grade,
				_class:      _class,
				number:      number,
			})
		}
	}

	// create request entity
	studentsForReq := make([]*authproto.BasicStudentInform, len(students))
	for i, student := range students {
		studentsForReq[i] = &authproto.BasicStudentInform{
			Name:          student.name,
			PhoneNumber:   student.phoneNumber,
			Grade:         uint32(student.grade),
			Group:         uint32(student._class),
			StudentNumber: uint32(student.number),
		}
	}

	// student 0개면 빠꾸

	// get service node
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AuthServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	// send gRPC request
	authSrvSpan := h.tracer.StartSpan("AddUnsignedStudents", opentracing.ChildOf(topSpan.Context()))
	ctxForReq := context.Background()
	ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
	ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
	rpcReq := &authproto.AddUnsignedStudentsRequest{
		UUID:     uuidClaims.UUID,
		Students: studentsForReq,
	}
	callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
	rpcResp, rpcErr := h.authService.AddUnsignedStudents(ctxForReq, rpcReq, callOpts...)
	authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
	authSrvSpan.Finish()

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for AddUnsignedStudents service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("AddUnsignedStudents returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("AddUnsignedStudents returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusCreated:
		status, _code := http.StatusCreated, 0
		sendResp := gin.H{"status": status, "code": _code, "message": rpcResp.Message,
			"req_student_count": len(studentsForReq), "no_add_count": rpcResp.NoAddCount, "add_count": rpcResp.AddCount}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": rpcResp.Message, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}
