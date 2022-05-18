package v1

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/httpclient"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/cihub/seelog"
)

// AgentMetadataPath is the Agent metadata path for v1 handler.
const (
	// ECSLogsCollectorPath is the Agent metadata path for v1 logs collector.
	ECSLogsCollectorPath = "/v1/logsbundle"

	logsFilePathDir = "/var/lib/ecs/data"

	s3UploadTimeout = 5 * time.Minute
)

type logCollectorResponse struct {
	LogBundleURL string
}

// ECSLogsCollectorHandler creates response for 'v1/logsbundle' API.
func ECSLogsCollectorHandler(containerInstanceArn, region string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// create logscollect state file
		createLogscollectFile()

		seelog.Infof("Finding the logsbundle...")
		logsFound, filename := isLogsCollectionSuccessful()
		if !logsFound {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
			return
		}
		defer os.Remove(filepath.Join(logsFilePathDir, filename))
		f, err := readLogsbundleTar(filepath.Join(logsFilePathDir, filename))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		defer f.Close()

		http.ServeContent(w, r, filename, time.Now(), f)

		// w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		// w.Header().Set("Content-Type", r.Header.Get("Content-Type"))

		// io.Copy(w, f)

		// upload the ecs logs
		// iamcredentials, err := instancecreds.GetCredentials(false).Get()
		// if err != nil {
		// 	seelog.Infof("Error getting instance credentials %v", err)
		// }
		// arn, err := arn.Parse(containerInstanceArn)
		// if err != nil {
		// 	seelog.Infof("Error parsing containerInstanceArn %s, err: %v", containerInstanceArn, err)
		// }
		// bucket := "ecs-logs-" + arn.AccountID
		// err = uploadECSLogsToS3(iamcredentials, bucket, key, region)
		// if err != nil {
		// 	seelog.Infof("Error uploading the ecs logs %v", err)
		// }

		// // return the presigned url
		// presignedUrl := getPreSignedUrl(iamcredentials, bucket, key, region)
		// seelog.Infof("Presigned URL for ECS logs: %s", presignedUrl)
		// http.Redirect(w, r, presignedUrl, 301)
		// // w.Write([]byte(presignedUrl))
		// // w.WriteHeader(http.StatusOK)
	}
}

func createLogscollectFile() {
	logscollectFile, err := os.Create(filepath.Join(logsFilePathDir, "logscollect"))
	if err != nil {
		seelog.Errorf("Error creating logscollect file, err: %v", err)
	}
	defer logscollectFile.Close()
	seelog.Infof("Successfully created %s file", logscollectFile.Name())
}

func readLogsbundleTar(logsbundlepath string) (*os.File, error) {
	seelog.Infof("Reading logsbundle from path %s", logsbundlepath)
	return os.Open(logsbundlepath)
}

func isLogsCollectionSuccessful() (bool, string) {
	var err error
	for i := 0; i < 60; i++ {
		matches, err := filepath.Glob(filepath.Join(logsFilePathDir, "collect-i*"))
		if err == nil && len(matches) > 0 {
			seelog.Infof("Found the logsbundle in %s", matches[0])
			filename := filepath.Base(matches[0])
			return true, filename
		}
		time.Sleep(1 * time.Second)
	}
	seelog.Errorf("Error while trying to find matches for %s, err: %v", filepath.Join(logsFilePathDir, "collect-i*"), err)
	return false, ""
}

func uploadECSLogsToS3(iamcredentials awscreds.Value, bucket, key, region string) error {
	//s3ClientCreator := factory.NewS3ClientCreator()
	cfg := aws.NewConfig().
		WithHTTPClient(httpclient.New(s3UploadTimeout, false)).
		WithCredentials(
			awscreds.NewStaticCredentials(iamcredentials.AccessKeyID, iamcredentials.SecretAccessKey,
				iamcredentials.SessionToken)).WithRegion(region)
	sess, err := session.NewSession(cfg)
	if err != nil {
		return err
	}

	// create the bucket if it does not exist
	svc := s3.New(sess)
	_, err = svc.HeadBucket(&s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			seelog.Infof("awserr received: %s", awsErr.Code())
			if awsErr.Code() == s3.ErrCodeNoSuchBucket || awsErr.Code() == "NotFound" {
				_, err = svc.CreateBucket(&s3.CreateBucketInput{
					Bucket: aws.String(bucket),
				})
				if err != nil {
					return fmt.Errorf("Error creating the bucket %s, err: %v", bucket, err)
				}
			} else {
				return err
			}
		} else {
			return err
		}
	}
	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)
	seelog.Infof("uploading the logsbundle")
	// Upload the file to S3.
	f, err := readLogsbundleTar(filepath.Join(logsFilePathDir, key))
	if err != nil {
		return err
	}
	defer f.Close()
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file, %v", err)
	}
	seelog.Infof("file uploaded to, %s\n", aws.StringValue(&result.Location))
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
	urlStr, err := req.Presign(1 * time.Hour)

	if err != nil {
		seelog.Errorf("Error to signing the request: %v", err)
	}

	return urlStr
}
