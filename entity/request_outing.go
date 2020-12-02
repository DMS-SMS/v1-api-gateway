package entity

import (
	outingproto "gateway/proto/golang/outing"
)

// request entity of GET /v1/clubs/paging
type GetStudentOutingsRequest struct {
	Start int32  `form:"start"`
	Count int32  `form:"count"`
}

func (from GetStudentOutingsRequest) GenerateGRPCRequest() (to *outingproto.GetStudentOutingsRequest) {
	to = new(outingproto.GetStudentOutingsRequest)
	to.Start = from.Start
	to.Count = from.Count
	return
}
