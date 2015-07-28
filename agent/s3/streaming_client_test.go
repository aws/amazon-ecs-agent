package s3

import (
	builtinBytes "bytes"
	"errors"
	"fmt"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/service/s3"
	"io/ioutil"
	"regexp"
	"strconv"
	"testing"
	"time"
)

const (
	bucket = "test-bucket"
	key    = "test/key"
)

// Test the case where S3 explicitly returns an error from the first HeadObject
// request.
func TestStreamObjectNoSuchObject(t *testing.T) {
	rawClient := NewMockRawS3Client()
	streamingClient := NewStreamingClient(rawClient)
	testTime := ttime.NewTestTime()
	ttime.SetTime(testTime)
	rawClient.headObject = func(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
		testTime.Warp(time.Minute)
		return nil, errors.New("Unable to find object")
	}
	_, err := streamingClient.StreamObject(bucket, key)

	if err == nil {
		t.Errorf("Exected an error to be raised")
	}

	return
}

// Test the case where S3 returns no content length.
func TestStreamObjectNoContentLength(t *testing.T) {
	rawClient := NewMockRawS3Client()
	streamingClient := NewStreamingClient(rawClient)
	testTime := ttime.NewTestTime()
	ttime.SetTime(testTime)
	rawClient.headObject = func(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
		testTime.Warp(time.Minute)
		var length int64 = 0
		return &s3.HeadObjectOutput{ContentLength: &length}, nil
	}

	_, err := streamingClient.StreamObject(bucket, key)

	if err == nil {
		t.Errorf("Expected error to be thrown")
	}

	return
}

// Test a successful stream
func TestStreamObjectSuccess(t *testing.T) {
	rawClient := NewMockRawS3Client()
	streamingClient := NewStreamingClient(rawClient)
	testTime := ttime.NewTestTime()
	ttime.SetTime(testTime)
	var contentLength int64 = (64 * 1024 * 1024) - 1

	rawClient.headObject = func(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
		if (input.Bucket == nil) || (*input.Bucket != bucket) {
			return nil, errors.New("Bad bucket")
		} else if (input.Key == nil) || (*input.Key != key) {
			return nil, errors.New("Bad key")
		}
		return &s3.HeadObjectOutput{ContentLength: &contentLength}, nil
	}

	rawClient.getObject = func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
		if (input.Bucket == nil) || (*input.Bucket != bucket) {
			return nil, errors.New("Bad bucket")
		} else if (input.Key == nil) || (*input.Key != key) {
			return nil, errors.New("Bad key")
		} else if input.Range == nil {
			return nil, errors.New("No range given")
		}

		regex, err := regexp.Compile("^bytes=(\\d+)-(\\d+)$")

		if err != nil {
			return nil, err
		}

		matches := regex.FindStringSubmatch(*input.Range)

		if len(matches) != 3 {
			message := fmt.Sprintf("Bad range: '%s'", *input.Range)
			return nil, errors.New(message)
		}

		begin, err := strconv.Atoi(matches[1])

		if err != nil {
			return nil, err
		}

		end, err := strconv.Atoi(matches[2])

		if err != nil {
			return nil, err
		}

		if int64(end) > contentLength {
			return nil, errors.New("Request out of bounds")
		}

		bytes := make([]byte, (end-begin)+1)

		for i := 0; i < len(bytes); i++ {
			bytes[i] = byte((begin + i) % 256)
		}

		return &s3.GetObjectOutput{Body: ioutil.NopCloser(builtinBytes.NewBuffer(bytes))}, nil
	}

	reader, err := streamingClient.StreamObject(bucket, key)

	if err != nil {
		t.Errorf("Error opening the reader: %s", err)
		return
	}

	bytes, err := ioutil.ReadAll(reader)
	if int64(len(bytes)) != contentLength {
		t.Errorf("Expected %i bytes, got %i", contentLength, len(bytes))
	}

	for i := 0; i < len(bytes); i++ {
		if bytes[i] != byte(i%256) {
			t.Errorf("Bad byte at index %i: %i. Expected %i.", i, bytes[i], i%256)
		}
	}

	err = reader.Close()

	if err != nil {
		t.Errorf("Error closing the reader: %s", err)
	}
}

// This struct is used to stub out requests to S3. Its HeadObject and GetObject
// fields may be set during tests to exercise the desired behavior.
type MockRawS3Client struct {
	headObject func(*s3.HeadObjectInput) (*s3.HeadObjectOutput, error)
	getObject  func(*s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

func NewMockRawS3Client() *MockRawS3Client {
	return &MockRawS3Client{
		headObject: func(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
			return nil, nil
		},
		getObject: func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			return nil, nil
		},
	}
}

func (client *MockRawS3Client) HeadObject(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
	return client.headObject(input)
}

func (client *MockRawS3Client) GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
	return client.getObject(input)
}
