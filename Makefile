
.PHONY: build
build: proto
	GOOS=linux GOARCH=amd64 go build -o api-gateway *.go

.PHONY: image
image:
	docker build . -t dms-sms-api-gateway:${VERSION}

.PHONY: upload
upload:
	docker tag dms-sms-api-gateway:${VERSION} jinhong0719/dms-sms-api-gateway:${VERSION}.RELEASE
	docker push jinhong0719/dms-sms-api-gateway:${VERSION}.RELEASE

.PHONY: pull
pull:
	docker pull jinhong0719/dms-sms-api-gateway:${VERSION}.RELEASE

.PHONY: run
run:
	docker-compose -f ./docker-compose.yml up -d

.PHONY: service
service:
	kubectl apply -f ./api-gateway-service.yaml

.PHONY: log_volume
log_volume:
	kubectl apply -f ./log-data-persistentvolume.yaml
	kubectl apply -f ./log-data-persistentvolumeclaim.yaml

.PHONY: api_gateway_deploy
deploy:
	envsubst < ./api-gateway-deployment.yaml | kubectl apply -f -

.PHONY: filebeat_run
filebeat_run:
	docker-compose -f ./filebeat-docker-compose.yml up -d

.PHONY: filebeat_deploy
filebeat_deploy:
	envsubst < ./filebeat-deployment.yaml | kubectl apply -f -

.PHONY: stack
stack:
	env VERSION=${VERSION} docker stack deploy -c docker-compose.yml DSM_SMS

.PHONY: filebeat_stack
filebeat_stack:
	docker stack deploy -c filebeat-docker-compose.yml DSM_SMS
