// Add file in v.1.0.4
// redis_handler_wrapper.go is file that defines method that returns pre-set redis handlers for each API

package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (r *redisHandler) CreateAnnouncement() []gin.HandlerFunc {
	redisDelKeys := []string{"announcements.uuid.*.types.$Type", "students.*.announcement-check", "writers.$TokenUUID.announcements"}
	return []gin.HandlerFunc{r.DeleteKeyEventPublisher(redisDelKeys, http.StatusCreated)}
}

func (r *redisHandler) GetAnnouncements() []gin.HandlerFunc {
	redisSetKey := "announcements.uuid.$TokenUUID.types.$type.start.$Start.count.$Count"
	return r.ResponderAndSetEventPublisher(redisSetKey, http.StatusOK)
}

func (r *redisHandler) GetAnnouncementDetail() []gin.HandlerFunc {
	redisDelKeys := []string{"students.$TokenUUID.announcement-check", "announcements.uuid.$TokenUUID.types.{announcements.$announcement_uuid.type}", "writers.$TokenUUID.announcements"}
	redisSetKey := "announcements.$announcement_uuid"
	return append([]gin.HandlerFunc{r.DeleteKeyEventPublisher(redisDelKeys, http.StatusOK)}, r.ResponderAndSetEventPublisher(redisSetKey, http.StatusOK)...)
}

func (r *redisHandler) UpdateAnnouncement() []gin.HandlerFunc {
	redisDelKeys := []string{"announcements.uuid.*.types.{announcements.$announcement_uuid.type}",
		"announcements.$announcement_uuid", "students.*.announcement-check", "writers.$TokenUUID.announcements"}
	return []gin.HandlerFunc{r.DeleteKeyEventPublisher(redisDelKeys, http.StatusOK)}
}

func (r *redisHandler) DeleteAnnouncement() []gin.HandlerFunc {
	redisDelKeys := []string{"announcements.uuid.*.types.{announcements.$announcement_uuid.type}",
		"announcements.$announcement_uuid", "students.*.announcement-check", "writers.$TokenUUID.announcements"}
	return []gin.HandlerFunc{r.DeleteKeyEventPublisher(redisDelKeys, http.StatusOK)}
}

func (r *redisHandler) CheckAnnouncement() []gin.HandlerFunc {
	redisSetKey := "students.$student_uuid.announcement-check"
	return r.ResponderAndSetEventPublisher(redisSetKey, http.StatusOK)
}

func (r *redisHandler) SearchAnnouncements() []gin.HandlerFunc {
	redisSetKey := "announcements.uuid.$TokenUUID.types.$type.query.$search_query.start.$Start.count,$Count"
	return r.ResponderAndSetEventPublisher(redisSetKey, http.StatusOK)
}

func (r *redisHandler) GetMyAnnouncements() []gin.HandlerFunc {
	redisSetKey := "writers.$writer_uuid.announcements"
	return r.ResponderAndSetEventPublisher(redisSetKey, http.StatusOK)
}
