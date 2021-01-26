// Add package in v.1.0.2
// listener is function that return closure used in subscribe
// aws_sqs.go is file that declare various closure about aws sqs like listening message, purging queue, etc ...

package subscriber

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	log "github.com/micro/go-micro/v2/logger"
	systemlog "log"
)

// function signature type for sqs message handler
type sqsMsgHandler func(*sqs.Message) error

// function that returns closure listening aws sqs message & handling with function receive from parameter
func SqsMsgListener(queue string, handler sqsMsgHandler, rcvInput *sqs.ReceiveMessageInput) func() {
	sqsSrv := sqs.New(awsSession)
	urlResult, err := sqsSrv.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(queue),
	})
	if err != nil {
		systemlog.Fatalf("unable to get queue url from queue name, name: %s, err:%v", queue, err)
	}

	if rcvInput == nil {
		rcvInput = &sqs.ReceiveMessageInput{}
	}
	rcvInput.QueueUrl = urlResult.QueueUrl

	return func() {
		var rcvOutput *sqs.ReceiveMessageOutput
		var err error
		var msg *sqs.Message

		for {
			rcvOutput, err = sqsSrv.ReceiveMessage(rcvInput)
			if err != nil {
				log.Errorf("some error occurs while pulling from aws sqs, queue: %s, err: %v", *rcvInput.QueueUrl, err)
				return
			}

			for _, msg = range rcvOutput.Messages {
				go func(msg *sqs.Message) {
					if err := handler(msg); err != nil {
						log.Errorf("some error occurs while handling aws sqs message, queue: %s, msg id: %s err: %v", *rcvInput.QueueUrl, *msg.MessageId, err)
					}
					if _, err := sqsSrv.DeleteMessage(&sqs.DeleteMessageInput{
						QueueUrl:      urlResult.QueueUrl,
						ReceiptHandle: msg.ReceiptHandle,
					}); err != nil {
						log.Errorf("some error occurs while deleting aws sqs message, queue: %s, msg id: %s err: %v", *rcvInput.QueueUrl, *msg.MessageId, err)
					}
				} (msg)
			}
		}
	}
}

// function that returns closure purging (deleting) all message in aws sqs queue
func SqsQueuePurger(queue string) func() {
	sqsSrv := sqs.New(awsSession)
	urlResult, err := sqsSrv.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(queue),
	})
	if err != nil {
		systemlog.Fatalf("unable to get queue url from queue name, name: %s, err:%v", queue, err)
	}

	purgeInput := &sqs.PurgeQueueInput{}
	purgeInput.QueueUrl = urlResult.QueueUrl

	return func() {
		if _, err := sqsSrv.PurgeQueue(purgeInput); err != nil {
			log.Errorf("some error occurs while deleting aws sqs message, queue: %s, err: %v", *purgeInput.QueueUrl, err)
		}
	}
}
