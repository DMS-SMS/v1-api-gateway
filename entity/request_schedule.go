package entity

import (
	scheduleproto "gateway/proto/golang/schedule"
)

// request entity of POST /v1/outings
type CreateScheduleRequest struct {
	StartDate int64  `json:"start_date" validate:"required,int_len=10"`
	EndDate   int64  `json:"end_date" validate:"required,int_len=10"`
	Detail    string `json:"detail" validate:"required,len=100"`
}

func (from CreateScheduleRequest) GenerateGRPCRequest() (to *scheduleproto.CreateScheduleRequest) {
	to = new(scheduleproto.CreateScheduleRequest)
	to.StartDate = from.StartDate
	to.EndDate = from.EndDate
	to.Detail = from.Detail
	return
}
