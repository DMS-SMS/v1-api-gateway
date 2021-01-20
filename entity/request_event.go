package entity

// request entity for GET /v1/clubs/uuid/:club_uuid/members
type PublishConsulChangeEventRequest struct {
	Node struct {
		ID      string `json:"ID"`
		Node    string `json:"Node"`
		Address string `json:"Address"`
	} `json:"Node"`

	Service struct {
		ID      string `json:"ID"`
		Service string `json:"Service"`
		Port    int    `json:"Port"`
		Address string `json:"Address"`
	} `json:"Service"`
}
