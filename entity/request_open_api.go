package entity

type GetPlaceWithNaverOpenAPIRequest struct {
	Keyword string `form:"keyword" validate:"required,max=100"`
}
