package main

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// S3 이벤트 알림 JSON 구조체 (SQS 메시지 Body 파싱용)
type S3EventWrapper struct {
	Records []struct {
		S3 struct {
			Bucket struct {
				Name string `json:"name"`
			} `json:"bucket"`
			Object struct {
				Key string `json:"key"`
			} `json:"object"`
		} `json:"s3"`
	} `json:"Records"`
}

func main() {
	// 1. AWS 설정 로드 (환경변수 AWS_ACCESS_KEY_ID 등 자동 인식)
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("AWS 설정 로드 실패: %v", err)
	}

	// 2. 클라이언트 생성
	sqsClient := sqs.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg)
	downloader := manager.NewDownloader(s3Client)

	// 환경변수에서 큐 URL 가져오기
	queueURL := os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		log.Fatal("SQS_QUEUE_URL 환경변수가 없습니다.")
	}

	log.Println("🚀 AI Worker 시작! 메시지 대기 중...")

	for {
		// 3. SQS 메시지 수신 (Long Polling)
		resp, err := sqsClient.ReceiveMessage(context.TODO(), &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     20, // 20초 동안 대기 (롱 폴링)
		})

		if err != nil {
			log.Printf("메시지 수신 에러: %v", err)
			continue
		}

		if len(resp.Messages) == 0 {
			continue // 메시지 없으면 다시 대기
		}

		for _, msg := range resp.Messages {
			log.Println("📩 메시지 수신됨!")
			processMessage(context.TODO(), downloader, sqsClient, queueURL, msg)
		}
	}
}

func processMessage(ctx context.Context, downloader *manager.Downloader, sqsClient *sqs.Client, queueURL string, msg types.Message) {
	// 4. 메시지 파싱 (S3 이벤트 정보 추출)
	var event S3EventWrapper
	if err := json.Unmarshal([]byte(*msg.Body), &event); err != nil {
		log.Printf("JSON 파싱 실패: %v", err)
		return
	}

	if len(event.Records) == 0 {
		log.Println("S3 이벤트 레코드가 없습니다. (테스트 메시지일 수 있음)")
		deleteMessage(ctx, sqsClient, queueURL, msg) // 그냥 삭제
		return
	}

	bucket := event.Records[0].S3.Bucket.Name
	rawKey := event.Records[0].S3.Object.Key

	key, err := url.QueryUnescape(rawKey)
	if err != nil {
		log.Printf("키 디코딩 실패: %v", err)
		return
	}

	log.Printf("🎯 타겟 발견: 버킷[%s] / 파일[%s]", bucket, key)

	// 5. S3 파일 다운로드
	file, err := os.Create("/tmp/" + key) // 로컬(컨테이너 내부)에 파일 생성
	if err != nil {
		log.Printf("파일 생성 실패: %v", err)
		return
	}
	defer file.Close()

	_, err = downloader.Download(ctx, file, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		log.Printf("❌ S3 다운로드 실패: %v", err)
		return
	}

	log.Printf("✅ 다운로드 완료: /tmp/%s", key)

	// (여기서 AI 처리 로직이 들어갑니다)

	// 6. 처리 완료 후 SQS 메시지 삭제 (필수!)
	deleteMessage(ctx, sqsClient, queueURL, msg)
}

func deleteMessage(ctx context.Context, client *sqs.Client, queueURL string, msg types.Message) {
	_, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.Printf("메시지 삭제 실패: %v", err)
	} else {
		log.Println("🗑️ SQS 메시지 삭제 완료 (처리 끝)")
	}
}
