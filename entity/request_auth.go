package entity

import (
	authproto "gateway/proto/golang/auth"
	"mime/multipart"
)

// request entity of POST /v1/students
type CreateNewStudentRequest struct {
	StudentID     string `form:"student_id" validate:"required,min=4,max=16"`
	StudentPW     string `form:"student_pw" validate:"required,min=4,max=16"`
	ParentUUID    string `form:"parent_uuid" validate:"required,uuid=parent,len=19"`
	Grade         int    `form:"grade" validate:"required,int_range=1~3"`
	Group         int    `form:"group" validate:"required,int_range=1~4"`
	StudentNumber int    `form:"student_number" validate:"required,int_range=1~21"`
	Name          string `form:"name" validate:"required,korean,min=2,max=4"`
	PhoneNumber   string `form:"phone_number" validate:"required,phone_number,len=11"`
	Profile       *multipart.FileHeader `form:"profile" validate:"required"`
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

	to.Image = make([]byte, from.Profile.Size)
	file, _ := from.Profile.Open()
	defer func() { _ = file.Close() }()
	_, _ = file.Read(to.Image)

	return
}

// request entity of POST /v1/teachers
type CreateNewTeacherRequest struct {
	TeacherID   string `form:"teacher_id" validate:"required,min=4,max=16"`
	TeacherPW   string `form:"teacher_pw" validate:"required,min=4,max=16"`
	Grade       int    `form:"grade" validate:"int_range=0~3"`
	Group       int    `form:"group" validate:"int_range=0~4"`
	Name        string `form:"name" validate:"required,korean,min=2,max=4"`
	PhoneNumber string `form:"phone_number" validate:"required,phone_number,len=11"`
}

func (from CreateNewTeacherRequest) GenerateGRPCRequest() (to *authproto.CreateNewTeacherRequest) {
	to = new(authproto.CreateNewTeacherRequest)
	to.TeacherID = from.TeacherID
	to.TeacherPW = from.TeacherPW
	to.Grade = uint32(from.Grade)
	to.Group = uint32(from.Group)
	to.Name = from.Name
	to.PhoneNumber = from.PhoneNumber
	return
}

// request entity of POST /v1/parents
type CreateNewParentRequest struct {
	ParentID    string `form:"parent_id" validate:"required,min=4,max=16"`
	ParentPW    string `form:"parent_pw" validate:"required,min=4,max=16"`
	Name        string `form:"name" validate:"required,korean,min=2,max=4"`
	PhoneNumber string `form:"phone_number" validate:"required,phone_number,len=11"`
}

func (from CreateNewParentRequest) GenerateGRPCRequest() (to *authproto.CreateNewParentRequest) {
	to = new(authproto.CreateNewParentRequest)
	to.ParentID = from.ParentID
	to.ParentPW = from.ParentPW
	to.Name = from.Name
	to.PhoneNumber = from.PhoneNumber
	return
}

// request entity of POST v1/login/admin
type LoginAdminAuthRequest struct {
	AdminID    string `form:"admin_id" validate:"required"`
	AdminPW    string `form:"admin_pw" validate:"required"`
}

func (from LoginAdminAuthRequest) GenerateGRPCRequest() (to *authproto.LoginAdminAuthRequest) {
	to = new(authproto.LoginAdminAuthRequest)
	to.AdminID = from.AdminID
	to.AdminPW = from.AdminPW
	return
}

// request entity of POST v1/login/student
type LoginStudentAuthRequest struct {
	StudentID    string `form:"student_id" validate:"required"`
	StudentPW    string `form:"student_pw" validate:"required"`
}

func (from LoginStudentAuthRequest) GenerateGRPCRequest() (to *authproto.LoginStudentAuthRequest) {
	to = new(authproto.LoginStudentAuthRequest)
	to.StudentID = from.StudentID
	to.StudentPW = from.StudentPW
	return
}

// request entity for PUT v1/students/{student_uuid}/password
type ChangeStudentPWRequest struct {
	CurrentPW   string `form:"current_pw" validate:"required"`
	RevisionPW  string `form:"revision_pw" validate:"required,min=4,max=16"`
}

func (from ChangeStudentPWRequest) GenerateGRPCRequest() (to *authproto.ChangeStudentPWRequest) {
	to = new(authproto.ChangeStudentPWRequest)
	to.CurrentPW = from.CurrentPW
	to.RevisionPW = from.RevisionPW
	return
}

// request entity for GET /v1/student-uuids
type GetStudentUUIDsWithInformRequest struct {
	Grade         int    `form:"grade" validate:"int_range=0~3"`
	Group         int    `form:"group" validate:"int_range=0~4"`
	StudentNumber int    `form:"student_number" validate:"int_range=0~21"`
	Name          string `form:"name" validate:"korean"`
	PhoneNumber   string `form:"phone_number" validate:"phone_number"`
	ProfileURI    string `form:"profile_uri"`
}

func (from GetStudentUUIDsWithInformRequest) GenerateGRPCRequest() (to *authproto.GetStudentUUIDsWithInformRequest) {
	to = new(authproto.GetStudentUUIDsWithInformRequest)
	to.Grade = uint32(from.Grade)
	to.Group = uint32(from.Group)
	to.StudentNumber = uint32(from.StudentNumber)
	to.Name = from.Name
	to.PhoneNumber = from.PhoneNumber
	to.ImageURI = from.ProfileURI
	return
}

// request entity of POST v1/login/teacher
type LoginTeacherAuthRequest struct {
	TeacherID string `form:"teacher_id" validate:"required"`
	TeacherPW string `form:"teacher_pw" validate:"required"`
}

func (from LoginTeacherAuthRequest) GenerateGRPCRequest() (to *authproto.LoginTeacherAuthRequest) {
	to = new(authproto.LoginTeacherAuthRequest)
	to.TeacherID = from.TeacherID
	to.TeacherPW = from.TeacherPW
	return
}

// request entity for PUT v1/teachers/{teacher_uuid}/password
type ChangeTeacherPWRequest struct {
	CurrentPW   string `form:"current_pw" validate:"required"`
	RevisionPW  string `form:"revision_pw" validate:"required,min=4,max=16"`
}

func (from ChangeTeacherPWRequest) GenerateGRPCRequest() (to *authproto.ChangeTeacherPWRequest) {
	to = new(authproto.ChangeTeacherPWRequest)
	to.CurrentPW = from.CurrentPW
	to.RevisionPW = from.RevisionPW
	return
}

// request entity for GET /v1/teacher-uuids
type GetTeacherUUIDsWithInformRequest struct {
	Grade         int    `form:"grade" validate:"int_range=0~3"`
	Group         int    `form:"group" validate:"int_range=0~4"`
	Name          string `form:"name" validate:"korean"`
	PhoneNumber   string `form:"phone_number" validate:"phone_number"`
}

func (from GetTeacherUUIDsWithInformRequest) GenerateGRPCRequest() (to *authproto.GetTeacherUUIDsWithInformRequest) {
	to = new(authproto.GetTeacherUUIDsWithInformRequest)
	to.Grade = uint32(from.Grade)
	to.Group = uint32(from.Group)
	to.Name = from.Name
	to.PhoneNumber = from.PhoneNumber
	return
}

// request entity of POST v1/login/parent
type LoginParentAuthRequest struct {
	ParentID string `form:"parent_id" validate:"required"`
	ParentPW string `form:"parent_pw" validate:"required"`
}

func (from LoginParentAuthRequest) GenerateGRPCRequest() (to *authproto.LoginParentAuthRequest) {
	to = new(authproto.LoginParentAuthRequest)
	to.ParentID = from.ParentID
	to.ParentPW = from.ParentPW
	return
}


// request entity for PUT v1/parents/{parent_uuid}/password
type ChangeParentPWRequest struct {
	CurrentPW   string `form:"current_pw" validate:"required"`
	RevisionPW  string `form:"revision_pw" validate:"required,min=4,max=16"`
}

func (from ChangeParentPWRequest) GenerateGRPCRequest() (to *authproto.ChangeParentPWRequest) {
	to = new(authproto.ChangeParentPWRequest)
	to.CurrentPW = from.CurrentPW
	to.RevisionPW = from.RevisionPW
	return
}

// request entity for GET /v1/parent-uuids
type GetParentUUIDsWithInformRequest struct {
	Name          string `form:"name" validate:"korean"`
	PhoneNumber   string `form:"phone_number" validate:"phone_number"`
}

func (from GetParentUUIDsWithInformRequest) GenerateGRPCRequest() (to *authproto.GetParentUUIDsWithInformRequest) {
	to = new(authproto.GetParentUUIDsWithInformRequest)
	to.Name = from.Name
	to.PhoneNumber = from.PhoneNumber
	return
}
