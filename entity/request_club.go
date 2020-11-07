package entity

import (
	clubproto "gateway/proto/golang/club"
	"mime/multipart"
	"strconv"
	"strings"
)

// request entity of POST /v1/clubs
type CreateNewClubRequest struct {
	Name        string `form:"name" validate:"required,max=30"`
	LeaderUUID  string `form:"leader_uuid" validate:"required,uuid=student,len=20"`
	MemberUUIDs string `form:"member_uuids" validate:"required"`
	Field       string `form:"field" validate:"required,max=20"`
	Location    string `form:"location" validate:"required,max=20"`
	Floor       int    `form:"floor" validate:"required,int_range=1~5"`
	Logo        *multipart.FileHeader `form:"logo" validate:"required"`
}

func (from CreateNewClubRequest) GenerateGRPCRequest() (to *clubproto.CreateNewClubRequest) {
	to = new(clubproto.CreateNewClubRequest)
	to.Name = from.Name
	to.LeaderUUID = from.LeaderUUID
	to.MemberUUIDs = strings.Split(from.MemberUUIDs, "|")
	to.Field = from.Field
	to.Location = from.Location
	to.Floor = strconv.Itoa(from.Floor)

	to.Logo = make([]byte, from.Logo.Size)
	file, _ := from.Logo.Open()
	defer func() { _ = file.Close() }()
	_, _ = file.Read(to.Logo)

	return
}

// request entity of GET /v1/clubs/paging
type GetClubsSortByUpdateTimeRequest struct {
	Start int    `form:"start"`
	Count int    `form:"count"`
	Field string `form:"field"`
	Name  string `form:"name"`
}

func (from GetClubsSortByUpdateTimeRequest) GenerateGRPCRequest() (to *clubproto.GetClubsSortByUpdateTimeRequest) {
	to = new(clubproto.GetClubsSortByUpdateTimeRequest)
	to.Start = uint32(from.Start)
	to.Count = uint32(from.Count)
	to.Field = from.Field
	to.Name = from.Name
	return
}

// request entity of GET /v1/recruitments/paging
type GetRecruitmentsSortByCreateTimeRequest struct {
	Start int    `form:"start"`
	Count int    `form:"count"`
	Field string `form:"field"`
	Name  string `form:"name"`
}

func (from GetRecruitmentsSortByCreateTimeRequest) GenerateGRPCRequest() (to *clubproto.GetRecruitmentsSortByCreateTimeRequest) {
	to = new(clubproto.GetRecruitmentsSortByCreateTimeRequest)
	to.Start = uint32(from.Start)
	to.Count = uint32(from.Count)
	to.Field = from.Field
	to.Name = from.Name
	return
}

// request entity for GET /v1/clubs
type GetClubInformsWithUUIDsRequest struct {
	ClubUUIDs []string `json:"club_uuids" validate:"required"`
}

func (from GetClubInformsWithUUIDsRequest) GenerateGRPCRequest() (to *clubproto.GetClubInformsWithUUIDsRequest) {
	to = new(clubproto.GetClubInformsWithUUIDsRequest)
	to.ClubUUIDs = from.ClubUUIDs
	return
}

// request entity for GET /v1/recruitment-uuids
type GetRecruitmentUUIDsWithClubUUIDsRequest struct {
	ClubUUIDs []string `json:"club_uuids" validate:"required"`
}

func (from GetRecruitmentUUIDsWithClubUUIDsRequest) GenerateGRPCRequest() (to *clubproto.GetRecruitmentUUIDsWithClubUUIDsRequest) {
	to = new(clubproto.GetRecruitmentUUIDsWithClubUUIDsRequest)
	to.ClubUUIDs = from.ClubUUIDs
	return
}

// request entity for GET /v1/clubs/uuid/:club_uuid/members
type AddClubMemberRequest struct {
	StudentUUID string `json:"student_uuid" validate:"required,uuid=student,len=20"`
}

func (from AddClubMemberRequest) GenerateGRPCRequest() (to *clubproto.AddClubMemberRequest) {
	to = new(clubproto.AddClubMemberRequest)
	to.StudentUUID = from.StudentUUID
	return
}

// request entity for PUT /v1/clubs/uuid/:club_uuid/leader
type ChangeClubLeaderRequest struct {
	NewLeaderUUID string `json:"new_leader_uuid" validate:"required,uuid=student,len=20"`
}

func (from ChangeClubLeaderRequest) GenerateGRPCRequest() (to *clubproto.ChangeClubLeaderRequest) {
	to = new(clubproto.ChangeClubLeaderRequest)
	to.NewLeaderUUID = from.NewLeaderUUID
	return
}

// request entity for PATCH /v1/clubs/uuid/:club_uuid/
type ModifyClubInformRequest struct {
	ClubConcept  string `form:"club_concept" validate:"max=40"`
	Introduction string `form:"introduction" validate:"max=150"`
	Link         string `form:"link" validate:"max=100"`
	Logo         *multipart.FileHeader `form:"logo"`
}

func (from ModifyClubInformRequest) GenerateGRPCRequest() (to *clubproto.ModifyClubInformRequest) {
	to = new(clubproto.ModifyClubInformRequest)
	to.ClubConcept = from.ClubConcept
	to.Introduction = from.Introduction
	to.Link = from.Link

	if from.Logo != nil {
		to.Logo = make([]byte, from.Logo.Size)
		file, _ := from.Logo.Open()
		defer func() { _ = file.Close() }()
		_, _ = file.Read(to.Logo)
	}

	return
}

// request entity for POST /v1/recruitments
type RegisterRecruitmentRequest struct {
	ClubUUID       string `json:"club_uuid" validate:"required"`
	RecruitConcept string `json:"recruit_concept" validate:"required,max=40"`
	EndPeriod      string `json:"end_period" validate:"time,max=10"`
	Members        []struct {
		Grade  int    `json:"grade" validate:"required,int_range=1~3"`
		Field  string `json:"field" validate:"required,max=20"`
		Number int    `json:"number" validate:"required,int_range=1~10"`
	} `json:"members" validate:"required"`
}

func (from RegisterRecruitmentRequest) GenerateGRPCRequest() (to *clubproto.RegisterRecruitmentRequest) {
	to = new(clubproto.RegisterRecruitmentRequest)
	to.ClubUUID = from.ClubUUID
	to.RecruitConcept = from.RecruitConcept
	to.EndPeriod = from.EndPeriod
	to.RecruitMembers = make([]*clubproto.RecruitMember, len(from.Members))
	for index, member := range from.Members {
		to.RecruitMembers[index] = &clubproto.RecruitMember{
			Grade:  strconv.Itoa(member.Grade),
			Field:  member.Field,
			Number: strconv.Itoa(member.Number),
		}
	}
	return
}

// request entity for PATCH /v1/recruitments/uuid/:recruitment_uuid
type ModifyRecruitmentRequest struct {
	RecruitConcept string `json:"recruit_concept" validate:"max=40"`
	EndPeriod      string `json:"end_period" validate:"time,max=10"`
	Members        []struct {
		Grade  int `json:"grade" validate:"required,int_range=1~3"`
		Field  string `json:"field" validate:"required,max=20"`
		Number int `json:"number" validate:"required,int_range=1~10"`
	} `json:"members"`
}

func (from ModifyRecruitmentRequest) GenerateGRPCRequest() (to *clubproto.ModifyRecruitmentRequest) {
	to = new(clubproto.ModifyRecruitmentRequest)
	to.RecruitConcept = from.RecruitConcept
	to.RecruitMembers = make([]*clubproto.RecruitMember, len(from.Members))
	for index, member := range from.Members {
		to.RecruitMembers[index] = &clubproto.RecruitMember{
			Grade:  strconv.Itoa(member.Grade),
			Field:  member.Field,
			Number: strconv.Itoa(member.Number),
		}
	}
	return
}
