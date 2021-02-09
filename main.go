package main

import (
	"context"
	"fmt"
	"gateway/consul"
	consulagent "gateway/consul/agent"
	"gateway/entity/validator"
	"gateway/handler"
	"gateway/middleware"
	announcementproto "gateway/proto/golang/announcement"
	authproto "gateway/proto/golang/auth"
	clubproto "gateway/proto/golang/club"
	outingproto "gateway/proto/golang/outing"
	scheduleproto "gateway/proto/golang/schedule"
	customrouter "gateway/router"
	"gateway/subscriber"
	"gateway/tool/env"
	topic "gateway/utils/topic/golang"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/bshuster-repo/logrus-logstash-hook"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/consul/api"
	"github.com/micro/go-micro/v2/client"
	grpccli "github.com/micro/go-micro/v2/client/grpc"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/transport/grpc"
	"github.com/sirupsen/logrus"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"log"
	"net/http"
	"os"
	"time"
)

// start profiling in this package init function (add in v.1.0.2)
import _ "gateway/tool/profiling"

func main() {
	// create consul connection & consul agent
	consulCfg := api.DefaultConfig()
	consulCfg.Address = env.GetAndFatalIfNotExits("CONSUL_ADDRESS") // change how to get env from local in v.1.0.2
	consulCli, err := api.NewClient(consulCfg)
	if err != nil {
		log.Fatalf("unable to connect consul agent, err: %v", err)
	}
	consulAgent := consulagent.Default(
		consulagent.Strategy(selector.RoundRobin),
		consulagent.Client(consulCli),
		consulagent.Services([]consul.ServiceName{topic.AuthServiceName, topic.ClubServiceName,  // add in v.1.0.2
			topic.OutingServiceName, topic.ScheduleServiceName, topic.AnnouncementServiceName}),
	)

	// create jaeger connection
	jaegerAddr := env.GetAndFatalIfNotExits("JAEGER_ADDRESS")
	apiTracer, closer, err := jaegercfg.Configuration{
		ServiceName: "DMS.SMS.v1.api.gateway", // add const in topic
		Reporter: &jaegercfg.ReporterConfig{LogSpans: true, LocalAgentHostPort: jaegerAddr},
		Sampler: &jaegercfg.SamplerConfig{Type: jaeger.SamplerTypeConst, Param: 1},
	}.NewTracer()
	if err != nil {
		log.Fatalf("error while creating new tracer for service, err: %v", err)
	}
	defer func() {
		_ = closer.Close()
	}()

	// create aws session (add in v.1.0.2)
	awsId := env.GetAndFatalIfNotExits("SMS_AWS_ID")
	awsKey := env.GetAndFatalIfNotExits("SMS_AWS_KEY")
	s3Region := env.GetAndFatalIfNotExits("SMS_AWS_REGION")
	awsSession, err := session.NewSession(&aws.Config{
		Region:      aws.String(s3Region),
		Credentials: credentials.NewStaticCredentials(awsId, awsKey, ""),
	})

	// create redis client (add in v.1.0.3)
	redisConf, err := consulAgent.GetRedisConfigFromKV("redis/gateway/local")
	if err != nil {
		log.Fatalf("unable to get redis connection config from consul KV, err: %v", err)
	}
	redisCli := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", redisConf.Host, redisConf.Port),
		DB:   redisConf.DB,
	})
	if err := redisCli.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("unable to connect to redis server, connection config: %v, err: %v", redisConf, err)
	}
	defer func() {
		_ = redisCli.Close()
	}()

	// gRPC service client
	gRPCCli := grpccli.NewClient(client.Transport(grpc.NewTransport()))
	authSrvCli := authproto.NewAuthService(topic.AuthServiceName, gRPCCli)
	clubSrvCli := clubproto.NewClubService(topic.ClubServiceName, gRPCCli)
	outingSrvCli := outingproto.NewOutingService("", gRPCCli)
	scheduleSrvCli := scheduleproto.NewScheduleService("schedule", gRPCCli)
	announcementSrvCli := announcementproto.NewAnnouncementService("announcement", gRPCCli)

	// create http request & event handler
	defaultHandler := handler.Default(
		handler.ConsulAgent(consulAgent),
		handler.Validate(validator.New()),
		handler.Tracer(apiTracer),
		handler.AWSSession(awsSession),
		handler.RedisClient(redisCli),
		handler.Location(time.UTC),
		handler.AuthService(authSrvCli),
		handler.ClubService(clubSrvCli),
		handler.OutingService(outingSrvCli),
		handler.ScheduleService(scheduleSrvCli),
		handler.AnnouncementService(announcementSrvCli),
	)

	// create subscriber & register aws sqs, redis listener (add in v.1.0.2)
	consulChangeQueue := env.GetAndFatalIfNotExits("CHANGE_CONSUL_SQS_GATEWAY")
	redisDeleteTopic := env.GetAndFatalIfNotExits("REDIS_DELETE_TOPIC")
	redisSetTopic := env.GetAndFatalIfNotExits("REDIS_SET_TOPIC")
	subscriber.SetAwsSession(awsSession)
	subscriber.SetRedisClient(redisCli)
	defaultSubscriber := subscriber.Default()
	defaultSubscriber.RegisterBeforeStart(
		subscriber.SqsQueuePurger(consulChangeQueue),
	)
	defaultSubscriber.RegisterListeners(
		subscriber.SqsMsgListener(consulChangeQueue, defaultHandler.ChangeConsulNodes, &sqs.ReceiveMessageInput{
			MaxNumberOfMessages: aws.Int64(10),
			WaitTimeSeconds:     aws.Int64(2),
		}),
		subscriber.RedisListener(redisDeleteTopic, defaultHandler.DeleteAssociatedRedisKey, 5), // add in v.1.0.3
		subscriber.RedisListener(redisSetTopic, defaultHandler.SetRedisKeyWithResponse, 5), // add in v.1.0.4
	)

	// create log file
	if _, err := os.Stat("/usr/share/filebeat/log/dms-sms"); os.IsNotExist(err) {
		if err = os.MkdirAll("/usr/share/filebeat/log/dms-sms", os.ModePerm); err != nil { log.Fatal(err) }
	}
	authLog, err := os.OpenFile("/usr/share/filebeat/log/dms-sms/auth.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil { log.Fatal(err) }
	clubLog, err := os.OpenFile("/usr/share/filebeat/log/dms-sms/club.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil { log.Fatal(err) }
	outingLog, err := os.OpenFile("/usr/share/filebeat/log/dms-sms/outing.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil { log.Fatal(err) }
	scheduleLog, err := os.OpenFile("/usr/share/filebeat/log/dms-sms/schedule.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil { log.Fatal(err) }
	announcementLog, err := os.OpenFile("/usr/share/filebeat/log/dms-sms/announcement.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil { log.Fatal(err) }
	openApiLog, err := os.OpenFile("/usr/share/filebeat/log/dms-sms/open-api.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil { log.Fatal(err) }

	// create logger & add hooks
	authLogger := logrus.New()
	authLogger.Hooks.Add(logrustash.New(authLog, logrustash.DefaultFormatter(logrus.Fields{"service": "auth"})))
	clubLogger := logrus.New()
	clubLogger.Hooks.Add(logrustash.New(clubLog, logrustash.DefaultFormatter(logrus.Fields{"service": "club"})))
	outingLogger := logrus.New()
	outingLogger.Hooks.Add(logrustash.New(outingLog, logrustash.DefaultFormatter(logrus.Fields{"service": "outing"})))
	scheduleLogger := logrus.New()
	scheduleLogger.Hooks.Add(logrustash.New(scheduleLog, logrustash.DefaultFormatter(logrus.Fields{"service": "schedule"})))
	announcementLogger := logrus.New()
	announcementLogger.Hooks.Add(logrustash.New(announcementLog, logrustash.DefaultFormatter(logrus.Fields{"service": "announcement"})))
	openApiLogger := logrus.New()
	openApiLogger.Hooks.Add(logrustash.New(openApiLog, logrustash.DefaultFormatter(logrus.Fields{"service": "open-api"})))

	// create custom router & register function to execute before run
	gin.SetMode(gin.ReleaseMode)
	globalRouter := customrouter.New(gin.Default())
	globalRouter.RegisterBeforeRun(
		defaultHandler.ConsulChangeEventPublisher(),
		consulAgent.ChangeAllServiceNodes,
		defaultSubscriber.StartListening,
	)

	// routing ping & pong API
	healthCheckRouter := globalRouter.Group("/")
	healthCheckRouter.GET("/ping", func(c *gin.Context) { // add in v.1.0.2
		c.JSON(http.StatusOK, "pong")
	})

	// routing API to use in consul watch
	consulWatchRouter := globalRouter.Group("/")
	consulWatchRouter.POST("/events/types/consul-change", defaultHandler.PublishConsulChangeEvent) // add in v.1.0.2

	// register middleware in global router & handler
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders, "Authorization", "authorization", "Request-Security")
	// run middleware before routing matching
	globalRouter.Use(
		cors.New(corsConfig),         // handle CORS request behind of AWS API Gateway
		middleware.SecurityFilter(),  // filter if verified client with algorithm using aes256
		middleware.Correlator(),      // set X-Request-ID field in request header to express correlate
		// middleware.DosDetector(),  // count request number per client IP to detect dos attack
	)
	// run middleware after successful routing matching
	router := globalRouter.CustomGroup("/",
		middleware.GinHResponseWriter(),          // change ResponseWriter in *gin.Context to custom writer overriding that (add in v.1.0.3)
		middleware.TracerSpanStarter(apiTracer),  // start, end top span of tracer & set log, tag about response (add in v.1.0.3)
	)
	router.Validator = validator.New()
	redisHandler := middleware.RedisHandler(redisCli, apiTracer, redisSetTopic)

	// routing auth service API
	authRouter := router.CustomGroup("/", middleware.LogEntrySetter(authLogger))
	// auth service api for admin
	authRouter.POSTWithAuth("/v1/students", defaultHandler.CreateNewStudent)
	authRouter.POSTWithAuth("/v1/teachers", defaultHandler.CreateNewTeacher)
	authRouter.POSTWithAuth("/v1/parents", defaultHandler.CreateNewParent)
	authRouter.POST("/v1/login/admin", defaultHandler.LoginAdminAuth)
	// auth service api for student
	authRouter.POST("/v1/login/student", defaultHandler.LoginStudentAuth)
	authRouter.PUTWithAuth("/v1/students/uuid/:student_uuid/password", defaultHandler.ChangeStudentPW)
	authRouter.GETWithAuth("/v1/students/uuid/:student_uuid", defaultHandler.GetStudentInformWithUUID)
	authRouter.GETWithAuth("/v1/student-uuids", defaultHandler.GetStudentUUIDsWithInform)
	authRouter.POSTWithAuth("/v1/students/with-uuids", defaultHandler.GetStudentInformsWithUUIDs)
	authRouter.GETWithAuth("/v1/students/uuid/:student_uuid/parent", defaultHandler.GetParentWithStudentUUID)
	// auth service api for teacher
	authRouter.POST("/v1/login/teacher", defaultHandler.LoginTeacherAuth)
	authRouter.PUTWithAuth("/v1/teachers/uuid/:teacher_uuid/password", defaultHandler.ChangeTeacherPW)
	authRouter.GETWithAuth("/v1/teachers/uuid/:teacher_uuid", defaultHandler.GetTeacherInformWithUUID)
	authRouter.GETWithAuth("/v1/teacher-uuids", defaultHandler.GetTeacherUUIDsWithInform)
	// auth service api for parent
	authRouter.POST("/v1/login/parent", defaultHandler.LoginParentAuth)
	authRouter.PUTWithAuth("/v1/parents/uuid/:parent_uuid/password", defaultHandler.ChangeParentPW)
	authRouter.GETWithAuth("/v1/parents/uuid/:parent_uuid", defaultHandler.GetParentInformWithUUID)
	authRouter.GETWithAuth("/v1/parent-uuids", defaultHandler.GetParentUUIDsWithInform)
	authRouter.GETWithAuth("/v1/parents/uuid/:parent_uuid/children", defaultHandler.GetChildrenInformsWithUUID)

	// routing club service API
	clubRouter := router.CustomGroup("/", middleware.LogEntrySetter(clubLogger))
	// club service api for admin
	clubRouter.POSTWithAuth("/v1/clubs", defaultHandler.CreateNewClub)
	// club service api for student
	clubRouter.GETWithAuth("/v1/clubs/sorted-by/update-time", defaultHandler.GetClubsSortByUpdateTime)
	clubRouter.GETWithAuth("/v1/recruitments/sorted-by/create-time", defaultHandler.GetRecruitmentsSortByCreateTime)
	clubRouter.GETWithAuth("/v1/clubs/uuid/:club_uuid", defaultHandler.GetClubInformWithUUID)
	clubRouter.GETWithAuth("/v1/clubs", defaultHandler.GetClubInformsWithUUIDs)
	clubRouter.GETWithAuth("/v1/recruitments/uuid/:recruitment_uuid", defaultHandler.GetRecruitmentInformWithUUID)
	clubRouter.GETWithAuth("/v1/clubs/uuid/:club_uuid/recruitment-uuid", defaultHandler.GetRecruitmentUUIDWithClubUUID)
	clubRouter.GETWithAuth("/v1/recruitment-uuids", defaultHandler.GetRecruitmentUUIDsWithClubUUIDs)
	clubRouter.GETWithAuth("/v1/clubs/property/fields", defaultHandler.GetAllClubFields)
	clubRouter.GETWithAuth("/v1/clubs/count", defaultHandler.GetTotalCountOfClubs)
	clubRouter.GETWithAuth("/v1/recruitments/count", defaultHandler.GetTotalCountOfCurrentRecruitments)
	clubRouter.GETWithAuth("/v1/leaders/uuid/:leader_uuid/club-uuid", defaultHandler.GetClubUUIDWithLeaderUUID)
	// club service api for club leader
	clubRouter.DELETEWithAuth("/v1/clubs/uuid/:club_uuid", defaultHandler.DeleteClubWithUUID)
	clubRouter.POSTWithAuth("/v1/clubs/uuid/:club_uuid/members", defaultHandler.AddClubMember)
	clubRouter.DELETEWithAuth("/v1/clubs/uuid/:club_uuid/members/:student_uuid", defaultHandler.DeleteClubMember)
	clubRouter.PUTWithAuth("/v1/clubs/uuid/:club_uuid/leader", defaultHandler.ChangeClubLeader)
	clubRouter.PATCHWithAuth("/v1/clubs/uuid/:club_uuid", defaultHandler.ModifyClubInform)
	clubRouter.POSTWithAuth("/v1/recruitments", defaultHandler.RegisterRecruitment)
	clubRouter.PATCHWithAuth("/v1/recruitments/uuid/:recruitment_uuid", defaultHandler.ModifyRecruitment)
	clubRouter.DELETEWithAuth("/v1/recruitments/uuid/:recruitment_uuid", defaultHandler.DeleteRecruitment)

	// routing outing service API
	outingRouter := router.CustomGroup("/", middleware.LogEntrySetter(outingLogger))
	outingRouter.POSTWithAuth("/v1/outings", defaultHandler.CreateOuting)
	outingRouter.GETWithAuth("/v1/students/uuid/:student_uuid/outings", defaultHandler.GetStudentOutings)
	outingRouter.GETWithAuth("/v1/outings/uuid/:outing_uuid", defaultHandler.GetOutingInform)
	outingRouter.GETWithAuth("/v1/outings/uuid/:outing_uuid/card", defaultHandler.GetCardAboutOuting)
	outingRouter.POST("/v1/outings/uuid/:outing_uuid/actions/:action", defaultHandler.TakeActionInOuting)
	outingRouter.GETWithAuth("/v1/outings/with-filter", defaultHandler.GetOutingWithFilter)
	outingRouter.GET("/v1/outings/code/:OCode", defaultHandler.GetOutingByOCode)

	// routing schedule service API
	scheduleRouter := router.CustomGroup("/", middleware.LogEntrySetter(scheduleLogger))
	scheduleRouter.POSTWithAuth("/v1/schedules", defaultHandler.CreateSchedule)
	scheduleRouter.GETWithAuth("/v1/schedules/years/:year/months/:month", defaultHandler.GetSchedule)
	scheduleRouter.GETWithAuth("/v1/time-tables/years/:year/months/:month/days/:day", defaultHandler.GetTimeTable)
	scheduleRouter.PATCHWithAuth("/v1/schedules/uuid/:schedule_uuid", defaultHandler.UpdateSchedule)
	scheduleRouter.DELETEWithAuth("/v1/schedules/uuid/:schedule_uuid", defaultHandler.DeleteSchedule)

	// routing announcement service API
	announcementRouter := router.CustomGroup("/", middleware.LogEntrySetter(announcementLogger))
	announcementRouter.POSTWithAuth("/v1/announcements", defaultHandler.CreateAnnouncement)
	announcementRouter.GETWithAuth("/v1/announcements/types/:type", defaultHandler.GetAnnouncements)
	announcementRouter.GETWithAuth("/v1/announcements/uuid/:announcement_uuid", defaultHandler.GetAnnouncementDetail)
	announcementRouter.PATCHWithAuth("/v1/announcements/uuid/:announcement_uuid", defaultHandler.UpdateAnnouncement)
	announcementRouter.DELETEWithAuth("/v1/announcements/uuid/:announcement_uuid", defaultHandler.DeleteAnnouncement)
	announcementRouter.GETWithAuth("/v1/students/uuid/:student_uuid/announcement-check", defaultHandler.CheckAnnouncement)
	announcementRouter.GETWithAuth("/v1/announcements/types/:type/query/:search_query", defaultHandler.SearchAnnouncements)
	announcementRouter.GETWithAuth("/v1/announcements/writer-uuid/:writer_uuid", defaultHandler.GetMyAnnouncements)

	// routing open-api agent API
	openApiRouter := router.CustomGroup("/", middleware.LogEntrySetter(openApiLogger))
	openApiRouter.GETWithAuth("/naver-open-api/search/local", defaultHandler.GetPlaceWithNaverOpenAPI)

	// run server
	log.Fatal(globalRouter.Run(":80"))
}
