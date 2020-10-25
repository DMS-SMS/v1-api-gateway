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
