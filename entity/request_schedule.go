package entity

import (
	scheduleproto "gateway/proto/golang/schedule"
)

// request entity of POST /v1/schedules
type CreateScheduleRequest struct {
	StartDate int64  `json:"start_date" validate:"required,int_len=10"`
	EndDate   int64  `json:"end_date" validate:"required,int_len=10"`
	Detail    string `json:"detail" validate:"required,max=100"`
}

func (from CreateScheduleRequest) GenerateGRPCRequest() (to *scheduleproto.CreateScheduleRequest) {
	to = new(scheduleproto.CreateScheduleRequest)
	to.StartDate = from.StartDate
	to.EndDate = from.EndDate
	to.Detail = from.Detail
	return
}

// request entity of GET /v1/schedules/years/:year/months/:month
type GetScheduleRequest struct {
	Year  int32 `uri:"year" validate:"required,int_range=0~9999"`
	Month int32 `uri:"month" validate:"required,int_range=1~12"`
}

func (from GetScheduleRequest) GenerateGRPCRequest() (to *scheduleproto.GetScheduleRequest) {
	to = new(scheduleproto.GetScheduleRequest)
	to.Year = from.Year
	to.Month = from.Month
	return
}

// request entity of GET /v1/time-tables/week-numbers/{week-number}
type GetTimeTableRequest struct {
	WeekNumber int32 `uri:"week-number" validate:"required,int_range=1~7"`
}

func (from GetTimeTableRequest) GenerateGRPCRequest() (to *scheduleproto.GetTimeTableRequest) {
	to = new(scheduleproto.GetTimeTableRequest)
	to.WeekNumber = from.WeekNumber
	return
}

// request entity of PATCH /v1/schedules/uuid/{schedule_uuid}
type UpdateScheduleRequest struct {
	StartDate int64  `json:"start_date" validate:"required,int_len=10"`
	EndDate   int64  `json:"end_date" validate:"required,int_len=10"`
	Detail    string `json:"detail" validate:"required,max=100"`
}

func (from UpdateScheduleRequest) GenerateGRPCRequest() (to *scheduleproto.UpdateScheduleRequest) {
	to = new(scheduleproto.UpdateScheduleRequest)
	to.StartDate = from.StartDate
	to.EndDate = from.EndDate
	to.Detail = from.Detail
	return
}
