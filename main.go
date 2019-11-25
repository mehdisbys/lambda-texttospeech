package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/polly"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/satori/go.uuid"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/session"
)

var bucket = os.Getenv("S3BUCKET")

// TTSRequest struct definition
type TTSRequest struct {
	TargetPolly     string  `json:"target_polly"`
	VoiceID         string  `json:"voice_id"`
	TextToTranslate *string `json:"text_to_translate"`
	ID              string  `json:"id"`
	S3Link          string  `json:"s3Link"`

	TTL        int64
	AudioText  io.ReadCloser    `json:"-"`
	AwsSession *session.Session `json:"-"`
}

type Response struct {
	S3Link string `json:"s3Link"`
}

func main() {
	lambda.Start(HandleRequest)
}

func HandleRequest(requestApi events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	req := TTSRequest{}
	err := json.Unmarshal([]byte(requestApi.Body), &req)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
	}

	log.Printf("body : %s", *req.TextToTranslate)

	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
		}))

	req.AwsSession = sess

	tts, err := TextToSpeech(&req)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
	}

	s3Link, err := saveToS3(*tts)
	if err != nil {
		log.Printf("error %s", err.Error())
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
	}

	log.Printf("sent speech to s3 %s ", s3Link)

	res := &Response{S3Link: s3Link}
	jres, err := json.Marshal(res)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 400}, nil
	}

	log.Printf("return response to gateway ")
	return events.APIGatewayProxyResponse{Body: string(jres), StatusCode: 200}, nil
}

func TextToSpeech(r *TTSRequest) (*TTSRequest, error) {
	// Create Polly client
	svc := polly.New(r.AwsSession)

	// Request text-to-speech conversion
	input := &polly.SynthesizeSpeechInput{
		LanguageCode: aws.String(r.TargetPolly),
		OutputFormat: aws.String("mp3"),
		Text:         r.TextToTranslate,
		VoiceId:      aws.String(r.VoiceID),
	}

	output, err := svc.SynthesizeSpeech(input)
	if err != nil {
		return nil, err
	}

	r.AudioText = output.AudioStream
	return r, nil
}

func saveToS3(r TTSRequest) (string, error) {
	svc := s3manager.NewUploader(r.AwsSession)

	r.setID()
	r.setTTL()

	filename := generateFilename(r.ID)
	log.Printf("s3 path %s", filename)

	output, err := svc.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(filename),
		Body:        r.AudioText,
		ContentType: aws.String("audio/mpeg"),
		ACL:         aws.String("public-read"),
	})

	if err != nil {
		log.Print(err.Error())
		return "", err
	}

	return output.Location, nil
}

func generateFilename(id string) string {
	now := time.Now()
	return fmt.Sprintf("%d/%s/%d/%s", now.Year(), now.Month().String(), now.Day(), id)
}

func (r *TTSRequest) setTTL() {
	r.TTL = time.Now().Add(time.Hour * 24 * 30).Unix()
}

func (r *TTSRequest) setID() {
	r.ID = uuid.NewV4().String()
}
