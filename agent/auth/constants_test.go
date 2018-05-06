// +build setup integ

package auth

const (
	// modify this to be the name of your --profile that has permission to call secret manager
	IntegAWSProfile = "petderek-laptop"
	// modify this to be your test region
	IntegRegion = "us-west-2"
	// you don't need to modify this if you are okay with this secret name
	IntegSecretsName = "secrettest"
)
