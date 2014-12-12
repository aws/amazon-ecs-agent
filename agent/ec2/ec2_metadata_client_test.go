package ec2

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

func MakeTestRoleCredentials() RoleCredentials {
	return RoleCredentials{
		Code:            "Success",
		LastUpdated:     time.Now(),
		Type:            "AWS-HMAC",
		AccessKeyId:     "ACCESSKEY",
		SecretAccessKey: "SECREKEY",
		Token:           "TOKEN",
		Expiration:      time.Now().Add(time.Duration(2 * time.Hour)),
	}
}

func ignoreError(v interface{}, _ error) interface{} {
	return v
}

const (
	TEST_ROLE_NAME = "test-role"
)

var test_client = EC2MetadataClient{client: testHttpClient{}}

var test_response = map[string]string{
	test_client.ResourceServiceUrl(SECURITY_CREDENTIALS_RESOURCE):                  TEST_ROLE_NAME,
	test_client.ResourceServiceUrl(SECURITY_CREDENTIALS_RESOURCE + TEST_ROLE_NAME): string(ignoreError(json.Marshal(MakeTestRoleCredentials())).([]byte)),
}

type testHttpClient struct{}

// Get is a mock of the http.Client.Get that reads its responses from the map
// above and defaults to erroring.
func (c testHttpClient) Get(url string) (*http.Response, error) {
	resp, ok := test_response[url]
	if ok {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Proto:      "HTTP/1.0",
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(resp))),
		}, nil
	}
	return nil, errors.New("404")
}

func TestDefaultCredentials(t *testing.T) {
	credentials, err := test_client.DefaultCredentials()
	if err != nil {
		t.Fail()
	}
	testCredentials := MakeTestRoleCredentials()
	if credentials.AccessKeyId != testCredentials.AccessKeyId {
		t.Fail()
	}
}
