package auth

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"

	"encoding/json"
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
)

type Auth struct {
	Username string `json:"username""`
	Password string `json:"password""`
}

type AuthProvider struct {
	secrets  *secretsmanager.SecretsManager
	secretid string
	region   string
}

func NewAuthProvider(sess *session.Session, region, secretid string) *AuthProvider {
	cfg := aws.NewConfig().WithRegion(region)
	client := secretsmanager.New(sess, cfg)

	return &AuthProvider{
		region:   region,
		secretid: secretid,
		secrets:  client,
	}
}

func (a *AuthProvider) GetAuth() Auth {
	if a.secrets == nil {
		log.Println("Secret client is empty")
		return EmptyAuth()
	}

	in := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(a.secretid),
	}

	out, err := a.secrets.GetSecretValue(in)
	if err != nil {
		log.Println("Error when making the secret request: ", err)
		return EmptyAuth()
	}

	auth := &Auth{}
	secretstring := aws.StringValue(out.SecretString)
	err = json.Unmarshal([]byte(secretstring), auth)
	if err != nil {
		log.Println("Json unable to unserialify: ", err)
		return EmptyAuth()
	}

	return *auth
}

func EmptyAuth() Auth {
	return Auth{"", ""}
}
