package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/credentials/instancecreds"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
)

const (
	ExternalInstanceCredentialsPath  = "/v1/external-instance-creds/"
	requestTypeExternalInstanceCreds = "external instance credentials"
)

type credentialsOutput struct {
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	SessionToken    string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
}

func ExternalInstanceCredentialsHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		creds, err := getCredentials()
		if err != nil {
			writeErrorResponse(w, err, "Error retrieving instance credentials")
			return
		}
		credentialsJSON, err := json.Marshal(creds)
		if err != nil {
			writeErrorResponse(w, err, "Error marshalling instance credentials")
			return
		}
		utils.WriteJSONToResponse(w, http.StatusOK, credentialsJSON, requestTypeExternalInstanceCreds)
		return
	}
}

func getCredentials() (credentialsOutput, error) {
	creds := instancecreds.GetCredentials()
	credsValue, err := creds.Get()
	if err != nil {
		return credentialsOutput{}, err
	}
	expires := time.Now().Add(time.Minute)
	return credentialsOutput{
		AccessKeyID:     credsValue.AccessKeyID,
		SecretAccessKey: credsValue.SecretAccessKey,
		SessionToken:    credsValue.SessionToken,
		Expiration:      expires,
	}, nil
}

func writeErrorResponse(w http.ResponseWriter, err error, errMessage string) {
	errResponseJSON, err := json.Marshal(fmt.Sprintf(errMessage+": %v", err.Error()))
	if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
		return
	}
	utils.WriteJSONToResponse(w, http.StatusInternalServerError, errResponseJSON, requestTypeExternalInstanceCreds)
}
