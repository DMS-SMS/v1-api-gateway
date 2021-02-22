// add file in v.1.0.3
// request_instance_sample.go is file that gather instance sample that set in request instance registry

package registry

import "gateway/entity"

var requestSamples = map[string]interface{}{
	// in "entity/request_announcement.go"
	"CreateAnnouncementRequest": entity.CreateAnnouncementRequest{},
	"GetAnnouncementsRequest": entity.GetAnnouncementsRequest{},
	"UpdateAnnouncementRequest": entity.UpdateAnnouncementRequest{},
	"SearchAnnouncementsRequest": entity.SearchAnnouncementsRequest{},
	"GetMyAnnouncementsRequest": entity.GetMyAnnouncementsRequest{},

	// in "entity/request_auth.go"
	"CreateNewStudentRequest": entity.CreateNewStudentRequest{},
	"CreateNewTeacherRequest": entity.CreateNewTeacherRequest{},
	"CreateNewParentRequest": entity.CreateNewParentRequest{},
	"LoginAdminAuthRequest": entity.LoginAdminAuthRequest{},
	"LoginStudentAuthRequest": entity.LoginStudentAuthRequest{},
	"ChangeStudentPWRequest": entity.ChangeStudentPWRequest{},
	"GetStudentUUIDsWithInformRequest": entity.GetStudentUUIDsWithInformRequest{},
	"GetStudentInformsWithUUIDsRequest": entity.GetStudentInformsWithUUIDsRequest{},
	"LoginTeacherAuthRequest": entity.LoginTeacherAuthRequest{},
	"ChangeTeacherPWRequest": entity.ChangeTeacherPWRequest{},
	"GetTeacherUUIDsWithInformRequest": entity.GetTeacherUUIDsWithInformRequest{},
	"LoginParentAuthRequest": entity.LoginParentAuthRequest{},
	"ChangeParentPWRequest": entity.ChangeParentPWRequest{},
	"GetParentUUIDsWithInformRequest": entity.GetParentUUIDsWithInformRequest{},
	"SendJoinSMSToUnsignedStudentsRequest": entity.SendJoinSMSToUnsignedStudentsRequest{},

	// in "entity/request_club.go"
	"CreateNewClubRequest": entity.CreateNewClubRequest{},
	"GetClubsSortByUpdateTimeRequest": entity.GetClubsSortByUpdateTimeRequest{},
	"GetRecruitmentsSortByCreateTimeRequest": entity.GetRecruitmentsSortByCreateTimeRequest{},
	"GetClubInformsWithUUIDsRequest": entity.GetClubInformsWithUUIDsRequest{},
	"GetRecruitmentUUIDsWithClubUUIDsRequest": entity.GetRecruitmentUUIDsWithClubUUIDsRequest{},
	"AddClubMemberRequest": entity.AddClubMemberRequest{},
	"ChangeClubLeaderRequest": entity.ChangeClubLeaderRequest{},
	"ModifyClubInformRequest": entity.ModifyClubInformRequest{},
	"RegisterRecruitmentRequest": entity.RegisterRecruitmentRequest{},
	"ModifyRecruitmentRequest": entity.ModifyRecruitmentRequest{},

	// in "entity/request_open_api.go"
	"GetPlaceWithNaverOpenAPIRequest": entity.GetPlaceWithNaverOpenAPIRequest{},

	// in "entity/request_outing.go"
	"CreateOutingRequest": entity.CreateOutingRequest{},
	"GetStudentOutingsRequest": entity.GetStudentOutingsRequest{},
	"GetOutingWithFilterRequest": entity.GetOutingWithFilterRequest{},

	// in "entity/request_schedule.go"
	"CreateScheduleRequest": entity.CreateScheduleRequest{},
	"GetScheduleRequest": entity.GetScheduleRequest{},
	"GetTimeTableRequest": entity.GetTimeTableRequest{},
	"UpdateScheduleRequest": entity.UpdateScheduleRequest{},

	// in "entity/request_xlsx.go"
	"AddUnsignedStudentsFromExcelRequest": entity.AddUnsignedStudentsFromExcelRequest{},
}
