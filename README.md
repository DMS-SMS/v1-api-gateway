# **DMS-SMS/V1**
> ### API Gateway 이외의 서비스들에 대한 Repository는 [**서비스 분해**](#서비스-분해) 부분에서 확인하실 수 있습니다!
## **INDEX**
### [**1. DMS-SMS란?**](#DMS-SMS란?)
### [**2. 서비스 기능**](#서비스-기능)
### [**3. 서비스 분해**](#서비스-분해)
### [**4. API Gateway 기능**](#API-Gateway-기능)
### [**5. 배포 방식**](#배포-방식)

<br>

## **[DMS-SMS](https://github.com/DMS-SMS)란?**
- SMS는 **School Management System**의 약어로, **학교 관리 시스템**을 의미합니다!

- **대덕소프트웨어마이스터고등학교**의 기숙사 관리 시스템(DMS, Dormitory Management System)을 개발하는 동아리인 **DMS**에서 **5기 부원들**(현 2020년 기준 2학년)이 개발하는 새로운 서비스입니다.
- **서버 개발**에는 현재 README.md를 작성중인 [**박진홍(PM)**](https://github.com/parkjinhong03)과 [**손민기**](https://github.com/mallycrip) 학생이 참여해 주었고, **MSA 기반**의 서버를 개발하였고 현재 운영중입니다.

<br>

## **서비스 기능**
1. ### **외출증 서비스**
    - 저희 서비스의 **주기능**으로, 오프라인 형식이였던 **기존의 외출 프로세스**를 **온라인 형식**으로 만든 것 입니다.
    - **학생**이 신청하고 **선생님 및 학부모**가 승인 및 조회를 할 수 있습니다.

2. ### **학사 일정 서비스**
    - **학사 일정 관련 정보**들을 관리하는 서비스로, 세부적으로는 **시간표**와 **캘린더** 관리 기능이 있습니다. 
    - **학생**이 볼 수 있고, 또한 캘린더는 **선생님**이 일정을 관리할 수도 있습니다.

3. ### **동아리 서비스**
    - 1학년 친구들 대상의 서비스로, **동아리 정보** 및 **동아리 모집 공고**를 관리하는 서비스입니다.
    - **동아리 관리자(부장)** 가 관리하고 **학생**이 조회할 수 있습니다.

4. ### **공지 서비스**
    - 마지막으로 **선생님** 및 **동아리 관리자(부장)** 가 관리하고 **학생**이 확인할 수 있는 **공지 서비스**가 있습니다. 
    - 참고로 선생님이 올리시는 학교 공지에는 **대상**(학년 반)을 설정할 수 있습니다.

<br>

## **서비스 분해**
> API Gateway에 대한 **HTTP API** 및 gRPC 서비스들에 대한 **RPC API**에 대해 **설계한 내용**들은 [**HTTP API 설계**](https://www.notion.so/HTTP-API-e6805bd5bf154c64bc367d7755badfc6)와 [**RPC API(Proto) 설계**](https://www.notion.so/RPC-API-Proto-064e1962f27643f3bdcdadb6307d545b)에서  추가로 확인하실 수 있습니다.

> 참고로, **protocol-buffer** 관련 파일들을 모와둔 [**레포지토리**](https://github.com/DMS-SMS/v1-protocol-buffer)에서도 참고하실 수 있습니다.  

1. ### [**API Gateway**](https://github.com/DMS-SMS/v1-api-gateway) *(개발 중)*
    - **이름** -> DMS.SMS.v1.api.gateway
    - **설명** -> 적절한 서비스에 **사용자 요청 라우팅** 서비스
    - **개발** -> 박진홍

2. ### [**Auth Service**](https://github.com/DMS-SMS/v1-service-auth) *(개발 완료)*
    - **이름** -> DMS.SMS.v1.service.auth
    - **설명** -> 학생, 선생님, 부모님 **계정 및 정보** 관리 서비스
    - **개발** -> 박진홍

3. ### [**Outing Service**](https://github.com/DMS-SMS/v1-service-outing) *(개발 중)*
    - **이름** -> DMS.SMS.v1.service.outing
    - **기능** -> **외출증** 관리 서비스
    - **개발** -> 손민기

4. ### [**Club Service**](https://github.com/DMS-SMS/v1-service-club) *(개발 완료)*
    - **이름** -> DMS.SMS.v1.service.club
    - **기능** -> **동아리(정보, 모집)** 관리 서비스
    - **개발** -> 박진홍

5. ### [**Announement Service**](https://github.com/DMS-SMS/v1-service-announcement) *(개발 전)*
    - **이름** -> DMS.SMS.v1.service.announcement
    - **기능** -> **공지(학교, 동아리)** 관리 서비스
    - **개발** -> 손민기

6. ### [**Schedule Service**](https://github.com/DMS-SMS/v1-service-schedule) *(개발 전)*
    - **이름** -> DMS.SMS.v1.service.schedule
    - **기능** -> **학사 일정(시간표, 캘린더)** 관리 서비스
    - **개발** -> 손민기

<br>

## **API Gateway 기능**
1. ### **요청 유효성 검사**
    - 서버에 전송한 payload에 꼭 필요한 **데이터가 다 존재**하는지, **데이터 제약조건**은 다 만족했는지 확인 *(x -> 400 Bad Request)* 

2. ### **인증 처리**
    - **토큰**이 헤더에 **존재**하고 **유효**한지, 해당 토큰의 payload의 uuid에 해당되는 **사용자가 존재하는지** 확인 *(X -> 401 Unauthorized)*
    - 호출한 서비스가 본인 계정의 권한으로 **접근이 가능한 서비스인지** 확인 *(X -> 403 Forbidden)*

3. ### **서비스 탐색 및 부하 분산**
    > 참고로 서비스의 **장애를 방지**하고 **크기를 동적으로 조정**하기 위해 이 **Service Discovery**를 **유연하게** 사용할 줄 알아야 함!
    - 서비스들이 **동적 IP 주소에 호스팅**되는 특성상 특정 서비스에 요청을 넣을 때 마다 **해당 서비스의 IP 주소**를 알아야하기 때문에 **Service Discovery**(consul 사용)에서 조회 *(조회 결과 X -> 503 Service Unavailable)*
    - 또한 서비스 **조회 결과가 여러 개**일 경우, 그 중 요청을 보낼 서비스를 정해야 하므로 **Load Balancing**이 필요함 *(Round Robin 알고리즘 사용)*

4. ### **계단식 오류 방지**
    > 특정 서비스에 **동기 통신**을 할 경우, 해당 서비스의 **응답이 무기한 연기**가 된다면 이는 곧 업스트림 서비스의 메모리까지 잡아먹게 되어 **계단식 오류가 발생**하기 때문에 이에 대한 대책이 필요함!
    - **응답**을 최대로 **대기**할 수 있는 **시간** 설정 *(시간 만료 -> 408 Request Timeout)*
    - 위와 같이 **특정 서비스가 불능 상태**가 되었을 경우, 해당 서비스로 요청이 가는 것을 차단하기 위해 **Circuit Breaker** 패턴 적용
        - **차단기 열림** -> Service Discovery의 Health Check을 Fatal로 변경 **(조회 X)**
        - **차단기 닫힘** -> Service Discovery의 Health Check을 Pass로 변경 **(조회 O)**

5. ### **Dos 공격 대비**
    - **동일한 IP**의 요청이 1초에 10개 이상 들어올 경우, **해당 IP 차단** 및 앞으로의 요청 **403 Forbidden 반환** *(10개 이상 들어온 시점 -> 429 Too Many Request)*

6. ### **관측성 패턴 적용**
    - **ELK Stack**(Elasticsearch + Logstash + Kibana + Filebeat)로 구성된 로그 시스템에 **로그 작성**
    - 외부 API에 대한 **지연 시간** 및 **응답 결과**를 작성하기 위한 **Distributed Trace**(jaeger 사용)를 시작하기 위해 Span 생성 및 Metadata로 다음 서비스에 Span-Context 전달

<br>

## **배포 방식**
- 각각의 서비스들은 **AWS EKS 클러스터**의 **노드**마다 하나씩 **Public Subnet**및 **Private Subnet**에 알맞게 배치되어 실행중입니다.

- 배포 서비스 업데이트 방식은 새로운 버전의 서비스 실행 파일이 담긴 Docker Image를 **Docker Hub에 올린** 후 AWS EKS 클러스터에 연결된 **kubectl 명령어**를 이용해서 **deployment 객체의 이미지 버전을 변경**합니다.
