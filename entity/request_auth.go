package entity

// request entity for POST /v1/clubs
type CreateNewStudentRequest struct {
	StudentID   string `json:"student_id" validate:"required,min=4,max=16"`
	StudentPW   string `json:"student_pw" validate:"required,min=4,max=16"`
	ParentUUID  string `json:"parent_uuid" validate:"required,uuid=parent,len=19"`
	Grade       int    `json:"grade" validate:"required,intRange=1~3"`
	Group       int    `json:"group" validate:"required,intRange=1~4"`
	Number      int    `json:"number" validate:"required,intRange=1~21"`
	Name        string `json:"name" validate:"required,korean,min=2,max=4"`
	PhoneNumber string `json:"phone_number" validate:"phone_number,len=11"`
}
