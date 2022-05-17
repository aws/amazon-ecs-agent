package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/aws-sdk-go/aws/arn"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/aws/amazon-ecs-agent/agent/credentials/instancecreds"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
)

// AgentMetadataPath is the Agent metadata path for v1 handler.
const (
	ECSLogsCollectorPath = "/v1/logsbundle"

	s3UploadTimeout = 5 * time.Minute
)

type logCollectorResponse struct {
	LogBundleURL string
}

// ECSLogsCollectorHandler creates response for 'v1/logsbundle' API.
func ECSLogsCollectorHandler(containerInstanceArn, region string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		// TODO: create a state file, if not exists
		logsFound := isLogsCollectionSuccessful()
		if !logsFound {
			// TODO: write error response
		}

		// TODO: parse the file name into key
		key := "instanceId"

		// upload the ecs logs
		iamcredentials, err := instancecreds.GetCredentials(false).Get()
		if err != nil {
			seelog.Debugf("Error getting instance credentials %v", err)
		}
		arn, err := arn.Parse(containerInstanceArn)
		if err != nil {
			seelog.Debugf("Error parsing containerInstanceArn %s, err: %v", containerInstanceArn, err)
		}
		bucket := "ecs-logs-" + arn.AccountID
		err = uploadECSLogsToS3(iamcredentials, bucket, key, region)
		if err != nil {
			seelog.Debugf("Error uploading the ecs logs %v", err)
		}

		// return the presigned url
		presignedUrl := getPreSignedUrl(iamcredentials, bucket, key, region)
		seelog.Infof("Presigned URL for ECS logs: %s", presignedUrl)
		resp := logCollectorResponse{LogBundleURL: presignedUrl}
		respBuf, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(respBuf)
	}
}

func isLogsCollectionSuccessful() bool {
	filepath := "/var/lib/ecs/data/i-*"
	for i := 0; i < 30; i++ {
		_, err := os.Stat(filepath)
		if err == nil {
			return true
		}
		time.Sleep(5 * time.Second)
	}
	return false
}

func uploadECSLogsToS3(iamcredentials awscreds.Value, bucket, key, region string) error {
	//s3ClientCreator := factory.NewS3ClientCreator()
	cfg := aws.NewConfig().
		WithHTTPClient(httpclient.New(s3UploadTimeout, false)).
		WithCredentials(
			awscreds.NewStaticCredentials(iamcredentials.AccessKeyID, iamcredentials.SecretAccessKey,
				iamcredentials.SessionToken)).WithRegion(region)
	sess := session.Must(session.NewSession(cfg))

	// create the bucket if it does not exist
	svc := s3.New(sess)
	_, err := svc.HeadBucket(&s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err.(awserr.Error).Code() == s3.ErrCodeNoSuchBucket {
		_, err = svc.CreateBucket(&s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			seelog.Errorf("Error creating the bucket %s, err: %v", bucket, err)
		}
	}
	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)
	// Upload the file to S3.
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte(`Hello`)),
	})
	if err != nil {
		return seelog.Errorf("failed to upload file, %v", err)
	}
	seelog.Debugf("file uploaded to, %s\n", aws.StringValue(&result.Location))
	/*s3Client, err := s3ClientCreator.NewS3ClientForECSLogsUpload(region, iamcredentials)
	err = agentS3.UploadFile("ecs-logs", "instanceId", []byte(`Hello`), s3UploadTimeout, s3Client)
	if err != nil {
		return err
	}*/
	return nil
}

func getPreSignedUrl(iamcredentials awscreds.Value, bucket, key, region string) string {
	cfg := aws.NewConfig().
		WithHTTPClient(httpclient.New(s3UploadTimeout, false)).
		WithCredentials(
			awscreds.NewStaticCredentials(iamcredentials.AccessKeyID, iamcredentials.SecretAccessKey,
				iamcredentials.SessionToken)).WithRegion(region)
	sess := session.Must(session.NewSession(cfg))

	// Create S3 service client
	svc := s3.New(sess)

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	urlStr, err := req.Presign(1 * time.Minute)

	if err != nil {
		seelog.Errorf("Error to signing the request: %v", err)
	}

	return urlStr
	/*	cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			log.Fatalf("Failed to load default config: %v", err)
		}
		client := s3.NewFromConfig(cfg)
		presignClient := s3.NewPresignClient(client)

		presignResult, err := presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String("ecs-logs"),
			Key:    aws.String("instanceId"),
		})

		if err != nil {
			panic("Couldn't get presigned URL for GetObject")
	}*/

}
