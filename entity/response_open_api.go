package entity

type GetPlaceWithNaverOpenAPIResponse struct {
	LastBuildDate string `json:"lastBuildDate"`
	Total         int    `json:"total"`
	Start         int    `json:"start"`
	Display       int    `json:"display"`
	Items         []struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Category    string `json:"category"`
		Telephone   string `json:"telephone"`
		Address     string `json:"address"`
		RoadAddress string `json:"roadAddress"`
		Mapx        string `json:"mapx"`
		Mapy        string `json:"mapy"`
	} `json:"items"`
	ErrorMessage string `json:"errorMessage"`
	ErrorCode    string `json:"errorCode"`
}
