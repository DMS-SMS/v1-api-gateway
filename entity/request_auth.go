package entity

import (
	authproto "gateway/proto/golang/auth"
)

// request entity of POST /v1/clubs
type CreateNewStudentRequest struct {
	StudentID     string `json:"student_id" validate:"required,min=4,max=16"`
	StudentPW     string `json:"student_pw" validate:"required,min=4,max=16"`
	ParentUUID    string `json:"parent_uuid" validate:"required,uuid=parent,len=19"`
	Grade         int    `json:"grade" validate:"required,int_range=1~3"`
	Group         int    `json:"group" validate:"required,int_range=1~4"`
	StudentNumber int    `json:"number" validate:"required,int_range=1~21"`
	Name          string `json:"name" validate:"required,korean,min=2,max=4"`
	PhoneNumber   string `json:"phone_number" validate:"phone_number,len=11"`
}

func (from CreateNewStudentRequest) GenerateGRPCRequest() (to *authproto.CreateNewStudentRequest) {
	to = new(authproto.CreateNewStudentRequest)
	to.StudentID = from.StudentID
	to.StudentPW = from.StudentPW
	to.ParentUUID = from.ParentUUID
	to.Grade = uint32(from.Grade)
	to.Group = uint32(from.Group)
	to.StudentNumber = uint32(from.StudentNumber)
	to.Name = from.Name
	to.PhoneNumber = from.PhoneNumber
	return
}
