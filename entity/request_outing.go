package entity

import (
	outingproto "gateway/proto/golang/outing"
)

// request entity of POST /v1/outings
type CreateOutingRequest struct {
	StartTime int64  `json:"start_time" validate:"required,int_len=10"`
	EndTime   int64  `json:"end_time" validate:"required,int_len=10"`
	Place     string `json:"place" validate:"required,max=50"`
	Reason    string `json:"reason" validate:"required,max=150"`
	Situation string `json:"situation" validate:"required,values=normal&emergency"`
}

func (from CreateOutingRequest) GenerateGRPCRequest() (to *outingproto.CreateOutingRequest) {
	to = new(outingproto.CreateOutingRequest)
	to.StartTime = from.StartTime
	to.EndTime = from.EndTime
	to.Place = from.Place
	to.Reason = from.Reason
	to.Situation = from.Situation
	return
}

// request entity of GET /v1/students/uuid/:student_uuid/outings
type GetStudentOutingsRequest struct {
	Start int32  `form:"start"`
	Count int32  `form:"count"`
}

func (from GetStudentOutingsRequest) GenerateGRPCRequest() (to *outingproto.GetStudentOutingsRequest) {
	if from.Count == 0 {
		from.Count = 10
	}

	to = new(outingproto.GetStudentOutingsRequest)
	to.Start = from.Start
	to.Count = from.Count
	return
}
