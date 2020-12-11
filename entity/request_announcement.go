package entity

import (
	announcementproto "gateway/proto/golang/announcement"
)

// request entity of POST /v1/announcements
type CreateAnnouncementRequest struct {
	Type        string `json:"type" validate:"required,values=school&club"`
	Title       string `json:"title" validate:"required,max=50"`
	Content     string `json:"content" validate:"required,max=1000"`
	TargetGrade int32  `json:"target_grade" validate:"int_range=1~3"`
	TargetGroup int32  `json:"target_group" validate:"int_range=1~4"`
}

func (from CreateAnnouncementRequest) GenerateGRPCRequest() (to *announcementproto.CreateAnnouncementRequest) {
	to = new(announcementproto.CreateAnnouncementRequest)
	to.Type = from.Type
	to.Title = from.Title
	to.Content = from.Content
	to.TargetGrade = from.TargetGrade
	to.TargetGroup = from.TargetGroup
	return
}

// request entity of GET /v1/students/uuid/:student_uuid/outings
type GetAnnouncementsRequest struct {
	Start int32  `form:"start"`
	Count int32  `form:"count"`
}

func (from GetAnnouncementsRequest) GenerateGRPCRequest() (to *announcementproto.GetAnnouncementsRequest) {
	if from.Count == 0 {
		from.Count = 10
	}

	to = new(announcementproto.GetAnnouncementsRequest)
	to.Start = from.Start
	to.Count = from.Count
	return
}