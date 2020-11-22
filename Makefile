
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

.PHONY: deploy
deploy:
	envsubst < ./api-gateway-deployment.yaml | kubectl apply -f -
