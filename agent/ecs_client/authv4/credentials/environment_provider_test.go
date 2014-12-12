package credentials

import (
	"os"
	"testing"
)

var fakeAccessKey = "12345678901234567890"
var fakeSecretKey = fakeAccessKey + fakeAccessKey
var fakeSessionToken = fakeSecretKey + fakeSecretKey

func TestCredentialsNotSet(t *testing.T) {
	os.Clearenv()
	_, err := new(EnvironmentCredentialProvider).Credentials()
	if err == nil {
		t.Fail()
	}
}

func TestCredentialsSet(t *testing.T) {
	os.Clearenv()
	os.Setenv("AWS_ACCESS_KEY_ID", fakeAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", fakeSecretKey)
	credentials, err := new(EnvironmentCredentialProvider).Credentials()
	if credentials.AccessKey != fakeAccessKey || credentials.SecretKey != fakeSecretKey || err != nil {
		t.Fail()
	}
}

func TestCredentialsWithSessionSet(t *testing.T) {
	os.Clearenv()
	os.Setenv("AWS_ACCESS_KEY_ID", fakeAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", fakeSecretKey)
	os.Setenv("AWS_SESSION_TOKEN", fakeSessionToken)
	credentials, err := new(EnvironmentCredentialProvider).Credentials()
	if credentials.AccessKey != fakeAccessKey || credentials.SecretKey != fakeSecretKey || credentials.Token != fakeSessionToken || err != nil {
		t.Fail()
	}
}
