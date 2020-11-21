package main

import (
	"gateway/entity/validator"
	"gateway/handler"
	"gateway/middleware"
	authproto "gateway/proto/golang/auth"
	clubproto "gateway/proto/golang/club"
	consulagent "gateway/tool/consul/agent"
	topic "gateway/utils/topic/golang"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/consul/api"
	"github.com/micro/go-micro/v2/client"
	grpccli "github.com/micro/go-micro/v2/client/grpc"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/transport/grpc"
	"github.com/sirupsen/logrus"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"log"
	"os"
)

func main() {
	// create consul connection
	consulCfg := api.DefaultConfig()
	consulCfg.Address = os.Getenv("CONSUL_ADDRESS")
	if consulCfg.Address == "" {
		log.Fatal("please set CONSUL_ADDRESS in environment variables")
	}
	consul, err := api.NewClient(consulCfg)
	if err != nil {
		log.Fatalf("unable to connect consul agent, err: %v", err)
	}
	consulAgent := consulagent.Default(
		consulagent.Client(consul),
		consulagent.Strategy(selector.RoundRobin),
	)

	// create jaeger connection
	jaegerAddr := os.Getenv("JAEGER_ADDRESS")
	if jaegerAddr == "" {
		log.Fatal("please set JAEGER_ADDRESS in environment variables")
	}
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

	// gRPC service client
	gRPCCli := grpccli.NewClient(client.Transport(grpc.NewTransport()))
	authSrvCli := struct {
		authproto.AuthAdminService
		authproto.AuthStudentService
		authproto.AuthTeacherService
		authproto.AuthParentService
	}{
		AuthAdminService:   authproto.NewAuthAdminService(topic.AuthServiceName, gRPCCli),
		AuthStudentService: authproto.NewAuthStudentService(topic.AuthServiceName, gRPCCli),
		AuthTeacherService: authproto.NewAuthTeacherService(topic.AuthServiceName, gRPCCli),
		AuthParentService:  authproto.NewAuthParentService(topic.AuthServiceName, gRPCCli),
	}
	clubSrvCli := struct {
		clubproto.ClubAdminService
		clubproto.ClubStudentService
		clubproto.ClubLeaderService
	}{
		ClubAdminService:   clubproto.NewClubAdminService(topic.ClubServiceName, gRPCCli),
		ClubStudentService: clubproto.NewClubStudentService(topic.ClubServiceName, gRPCCli),
		ClubLeaderService:  clubproto.NewClubLeaderService(topic.ClubServiceName, gRPCCli),
	}

	// create http request handler
	httpHandler := handler.Default(
		handler.ConsulAgent(consulAgent),
		handler.Validate(validator.New()),
		handler.Tracer(apiTracer),
		handler.AuthService(authSrvCli),
		handler.ClubService(clubSrvCli),
	)

	router := gin.Default()
	router.Use(cors.Default(), middleware.DosDetector(), middleware.Correlator(), middleware.LogEntrySetter(logrus.New()))

	// auth service api for admin
	router.POST("/v1/students", httpHandler.CreateNewStudent)
	router.POST("/v1/teachers", httpHandler.CreateNewTeacher)
	router.POST("/v1/parents", httpHandler.CreateNewParent)
	router.POST("/v1/login/admin", httpHandler.LoginAdminAuth)

	// auth service api for student
	router.POST("/v1/login/student", httpHandler.LoginStudentAuth)
	router.PUT("/v1/students/uuid/:student_uuid/password", httpHandler.ChangeStudentPW)
	router.GET("/v1/students/uuid/:student_uuid", httpHandler.GetStudentInformWithUUID)
	router.GET("/v1/student-uuids", httpHandler.GetStudentUUIDsWithInform)
	router.GET("/v1/students", httpHandler.GetStudentInformsWithUUIDs)

	// auth service api for teacher
	router.POST("/v1/login/teacher", httpHandler.LoginTeacherAuth)
	router.PUT("/v1/teachers/uuid/:teacher_uuid/password", httpHandler.ChangeTeacherPW)
	router.GET("/v1/teachers/uuid/:teacher_uuid", httpHandler.GetTeacherInformWithUUID)
	router.GET("/v1/teacher-uuids", httpHandler.GetTeacherUUIDsWithInform)

	// auth service api for parent
	router.POST("/v1/login/parent", httpHandler.LoginParentAuth)
	router.PUT("/v1/parents/uuid/:parent_uuid/password", httpHandler.ChangeParentPW)
	router.GET("/v1/parents/uuid/:parent_uuid", httpHandler.GetParentInformWithUUID)
	router.GET("/v1/parent-uuids", httpHandler.GetParentUUIDsWithInform)

	// club service api for admin
	router.POST("/v1/clubs", httpHandler.CreateNewClub)

	// club service api for student
	router.GET("/v1/clubs/sorted-by/update-time", httpHandler.GetClubsSortByUpdateTime)
	router.GET("/v1/recruitments/sorted-by/create-time", httpHandler.GetRecruitmentsSortByCreateTime)
	router.GET("/v1/clubs/uuid/:club_uuid", httpHandler.GetClubInformWithUUID)
	router.GET("/v1/clubs", httpHandler.GetClubInformsWithUUIDs)
	router.GET("/v1/recruitments/uuid/:recruitment_uuid", httpHandler.GetRecruitmentInformWithUUID)
	router.GET("/v1/clubs/uuid/:club_uuid/recruitment-uuid", httpHandler.GetRecruitmentUUIDWithClubUUID)
	router.GET("/v1/recruitment-uuids", httpHandler.GetRecruitmentUUIDsWithClubUUIDs)
	router.GET("/v1/clubs/property/fields", httpHandler.GetAllClubFields)
	router.GET("/v1/clubs/count", httpHandler.GetTotalCountOfClubs)
	router.GET("/v1/recruitments/count", httpHandler.GetTotalCountOfCurrentRecruitments)
	router.GET("/v1/leaders/uuid/:leader_uuid/club-uuid", httpHandler.GetClubUUIDWithLeaderUUID)

	// club service api for club leader
	router.POST("/v1/clubs/uuid/:club_uuid/members", httpHandler.AddClubMember)
	router.DELETE("/v1/clubs/uuid/:club_uuid/members/:student_uuid", httpHandler.DeleteClubMember)
	router.PUT("/v1/clubs/uuid/:club_uuid/leader", httpHandler.ChangeClubLeader)
	router.PATCH("/v1/clubs/uuid/:club_uuid", httpHandler.ModifyClubInform)
	router.DELETE("/v1/clubs/uuid/:club_uuid", httpHandler.DeleteClubWithUUID)
	router.POST("/v1/recruitments", httpHandler.RegisterRecruitment)
	router.PATCH("/v1/recruitments/uuid/:recruitment_uuid", httpHandler.ModifyRecruitment)
	router.DELETE("/v1/recruitments/uuid/:recruitment_uuid", httpHandler.DeleteRecruitment)

	log.Fatal(router.Run(":8080"))
}
