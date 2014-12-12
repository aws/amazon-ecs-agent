package credentials

func NewCredentialProvider(access, secret string) AWSCredentialProvider {
	return &AWSCredentials{AccessKey: access, SecretKey: secret, Token: ""}
}
