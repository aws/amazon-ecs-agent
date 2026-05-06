// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.
package gmsacredclient

import (
	"context"
	"os"
	"time"

	pb "github.com/aws/amazon-ecs-agent/ecs-agent/gmsacredclient/credentialsfetcher"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Retry policy for gRPC calls to the credentials fetcher daemon.
const (
	grpcCallRetryAttempts        = 3
	grpcCallRetryBackoffMin      = 1 * time.Second
	grpcCallRetryBackoffMax      = 5 * time.Second
	grpcCallRetryBackoffJitter   = 0.2
	grpcCallRetryBackoffMultiple = 2.0
)

type CredentialsFetcherClient struct {
	conn    *grpc.ClientConn
	timeout time.Duration
	backoff retry.Backoff
}

// GetGrpcClientConnection() returns grpc client connection
func GetGrpcClientConnection() (*grpc.ClientConn, error) {
	address, err := getSocketAddress()
	if err != nil {
		logger.Error("could not find path to credentials fetcher host dir", logger.Fields{field.Error: err})
		return nil, err
	}

	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("could not initialize client connection", logger.Fields{field.Error: err})
		return nil, err
	}
	return conn, nil

}

// getSocketAddress() returns the credentials-fetcher socket dir
func getSocketAddress() (string, error) {
	credentialsfetcherHostDir := os.Getenv("CREDENTIALS_FETCHER_HOST_DIR")

	_, err := os.Stat(credentialsfetcherHostDir)
	if err != nil {
		return "", err
	}
	return "unix:" + credentialsfetcherHostDir, nil
}

func NewCredentialsFetcherClient(conn *grpc.ClientConn, timeout time.Duration) CredentialsFetcherClient {
	return CredentialsFetcherClient{
		conn:    conn,
		timeout: timeout,
		backoff: retry.NewExponentialBackoff(
			grpcCallRetryBackoffMin,
			grpcCallRetryBackoffMax,
			grpcCallRetryBackoffJitter,
			grpcCallRetryBackoffMultiple,
		),
	}
}

// Credentials fetcher is a daemon running on the host which supports gMSA on linux
type CredentialsFetcherResponse struct {
	//lease id is a unique identifier associated with the kerberos tickets created for a container
	LeaseID string
	//path to the kerberos tickets created for the service accounts
	KerberosTicketPaths []string
}

// Credentials fetcher is a daemon running on the host which supports gMSA on linux
type CredentialsFetcherArnResponse struct {
	//lease id is a unique identifier associated with the kerberos tickets created for a container
	LeaseID string
	//path to the kerberos tickets created for the service accounts
	KerberosTicketsMap map[string]string
}

// HealthCheck() invokes credentials fetcher daemon running on the host
// to check the health status of daemon
func (c CredentialsFetcherClient) HealthCheck(ctx context.Context, serviceName string) (string, error) {
	if len(serviceName) == 0 {
		return "", status.Errorf(codes.InvalidArgument, "service name should not be empty")
	}
	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.HealthCheckRequest{Service: serviceName}

	response, attempts, err := retry.CallWithRetry(ctx, c.backoff, grpcCallRetryAttempts, c.timeout, func(callCtx context.Context) (*pb.HealthCheckResponse, error) {
		return client.HealthCheck(callCtx, request)
	})
	if err != nil {
		return "", err
	}
	logger.Debug("credentials-fetcher daemon is running", logger.Fields{field.Attempts: attempts})

	return response.GetStatus(), nil
}

// AddKerberosArnLease() invokes credentials fetcher daemon running on the host
// to create kerberos tickets associated with gMSA accounts
func (c CredentialsFetcherClient) AddKerberosArnLease(ctx context.Context, credentialspecsArns []string, accessKeyId string, secretKey string, sessionToken string, region string) (CredentialsFetcherArnResponse, error) {
	if len(credentialspecsArns) == 0 {
		return CredentialsFetcherArnResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecArns should not be empty")
	}

	if len(accessKeyId) == 0 || len(secretKey) == 0 || len(sessionToken) == 0 || len(region) == 0 {
		return CredentialsFetcherArnResponse{}, status.Errorf(codes.InvalidArgument, "accessid, secretkey, sessiontoken or region should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.KerberosArnLeaseRequest{CredspecArns: credentialspecsArns, AccessKeyId: accessKeyId, SecretAccessKey: secretKey, SessionToken: sessionToken, Region: region}

	response, attempts, err := retry.CallWithRetry(ctx, c.backoff, grpcCallRetryAttempts, c.timeout, func(callCtx context.Context) (*pb.CreateKerberosArnLeaseResponse, error) {
		return client.AddKerberosArnLease(callCtx, request)
	})
	if err != nil {
		return CredentialsFetcherArnResponse{}, err
	}
	logger.Info("created kerberos tickets", logger.Fields{field.LeaseID: response.GetLeaseId(), field.Attempts: attempts})

	credentialsFetcherResponse := CredentialsFetcherArnResponse{
		LeaseID:            response.GetLeaseId(),
		KerberosTicketsMap: make(map[string]string),
	}

	for _, value := range response.GetKrbTicketResponseMap() {
		credSpecArns := value.GetCredspecArns()
		_, ok := credentialsFetcherResponse.KerberosTicketsMap[credSpecArns]
		if !ok {
			credentialsFetcherResponse.KerberosTicketsMap[credSpecArns] = value.GetCreatedKerberosFilePaths()
		}
	}

	return credentialsFetcherResponse, nil
}

// RenewKerberosArnLease() invokes credentials fetcher daemon running on the host
// to renew kerberos tickets associated with gMSA accounts
func (c CredentialsFetcherClient) RenewKerberosArnLease(ctx context.Context, accessKeyId string, secretKey string, sessionToken string, region string) (string, error) {
	if len(accessKeyId) == 0 || len(secretKey) == 0 || len(sessionToken) == 0 || len(region) == 0 {
		return codes.InvalidArgument.String(), status.Errorf(codes.InvalidArgument, "accessid, secretkey, sessiontoken or region should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.RenewKerberosArnLeaseRequest{AccessKeyId: accessKeyId, SecretAccessKey: secretKey, SessionToken: sessionToken, Region: region}

	response, attempts, err := retry.CallWithRetry(ctx, c.backoff, grpcCallRetryAttempts, c.timeout, func(callCtx context.Context) (*pb.RenewKerberosArnLeaseResponse, error) {
		return client.RenewKerberosArnLease(callCtx, request)
	})
	if err != nil {
		return codes.Internal.String(), err
	}

	if response.GetStatus() == "failed" {
		return codes.Internal.String(), status.Errorf(codes.Internal, "renewal of kerberos tickets failed")
	}

	logger.Info("renewal of kerberos tickets are successful", logger.Fields{field.Attempts: attempts})

	return codes.OK.String(), nil
}

// AddKerberosLease() invokes credentials fetcher daemon running on the host
// to create kerberos tickets associated with gMSA accounts
func (c CredentialsFetcherClient) AddKerberosLease(ctx context.Context, credentialspecs []string) (CredentialsFetcherResponse, error) {
	if len(credentialspecs) == 0 {
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecs should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.CreateKerberosLeaseRequest{CredspecContents: credentialspecs}

	response, attempts, err := retry.CallWithRetry(ctx, c.backoff, grpcCallRetryAttempts, c.timeout, func(callCtx context.Context) (*pb.CreateKerberosLeaseResponse, error) {
		return client.AddKerberosLease(callCtx, request)
	})
	if err != nil {
		return CredentialsFetcherResponse{}, err
	}
	logger.Info("created kerberos tickets", logger.Fields{field.LeaseID: response.GetLeaseId(), field.Attempts: attempts})

	credentialsFetcherResponse := CredentialsFetcherResponse{
		LeaseID:             response.GetLeaseId(),
		KerberosTicketPaths: response.GetCreatedKerberosFilePaths(),
	}

	return credentialsFetcherResponse, nil
}

// AddNonDomainJoinedKerberosLease() invokes credentials fetcher daemon running on the host
// to create kerberos tickets associated with gMSA accounts in domainless mode
func (c CredentialsFetcherClient) AddNonDomainJoinedKerberosLease(ctx context.Context, credentialspecs []string, username string, password string, domain string) (CredentialsFetcherResponse, error) {
	if len(credentialspecs) == 0 {
		logger.Error("credentialspecs request should not be empty")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecs should not be empty")
	}

	if len(username) == 0 || len(password) == 0 || len(domain) == 0 {
		logger.Error("username, password or domain should not be empty")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "username, password or domain should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.CreateNonDomainJoinedKerberosLeaseRequest{CredspecContents: credentialspecs, Username: username, Password: password, Domain: domain}

	response, attempts, err := retry.CallWithRetry(ctx, c.backoff, grpcCallRetryAttempts, c.timeout, func(callCtx context.Context) (*pb.CreateNonDomainJoinedKerberosLeaseResponse, error) {
		return client.AddNonDomainJoinedKerberosLease(callCtx, request)
	})
	if err != nil {
		return CredentialsFetcherResponse{}, err
	}
	logger.Info("created kerberos tickets", logger.Fields{field.LeaseID: response.GetLeaseId(), field.Attempts: attempts})

	credentialsFetcherResponse := CredentialsFetcherResponse{
		LeaseID:             response.GetLeaseId(),
		KerberosTicketPaths: response.GetCreatedKerberosFilePaths(),
	}

	return credentialsFetcherResponse, nil
}

// RenewNonDomainJoinedKerberosLease() invokes credentials fetcher daemon running on the host
// to renew kerberos tickets associated with gMSA accounts in domainless mode
func (c CredentialsFetcherClient) RenewNonDomainJoinedKerberosLease(ctx context.Context, username string, password string, domain string) (CredentialsFetcherResponse, error) {
	if len(username) == 0 || len(password) == 0 || len(domain) == 0 {
		logger.Error("username, password or domain should not be empty")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "username, password or domain should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.RenewNonDomainJoinedKerberosLeaseRequest{Username: username, Password: password, Domain: domain}

	response, attempts, err := retry.CallWithRetry(ctx, c.backoff, grpcCallRetryAttempts, c.timeout, func(callCtx context.Context) (*pb.RenewNonDomainJoinedKerberosLeaseResponse, error) {
		return client.RenewNonDomainJoinedKerberosLease(callCtx, request)
	})
	if err != nil {
		return CredentialsFetcherResponse{}, err
	}
	logger.Info("renewed kerberos tickets", logger.Fields{field.Attempts: attempts})

	credentialsFetcherResponse := CredentialsFetcherResponse{
		KerberosTicketPaths: response.GetRenewedKerberosFilePaths(),
	}

	return credentialsFetcherResponse, nil
}

// DeleteKerberosLease() invokes credentials fetcher daemon running on the host
// to delete kerberos tickets of gMSA accounts associated with the leaseid
func (c CredentialsFetcherClient) DeleteKerberosLease(ctx context.Context, leaseid string) (CredentialsFetcherResponse, error) {
	if len(leaseid) == 0 {
		logger.Error("invalid leaseid provided")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "invalid leaseid provided")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.DeleteKerberosLeaseRequest{LeaseId: leaseid}

	response, attempts, err := retry.CallWithRetry(ctx, c.backoff, grpcCallRetryAttempts, c.timeout, func(callCtx context.Context) (*pb.DeleteKerberosLeaseResponse, error) {
		return client.DeleteKerberosLease(callCtx, request)
	})
	if err != nil {
		return CredentialsFetcherResponse{}, err
	}
	logger.Info("deleted kerberos tickets", logger.Fields{field.LeaseID: response.GetLeaseId(), field.Attempts: attempts})

	credentialsFetcherResponse := CredentialsFetcherResponse{
		LeaseID:             response.GetLeaseId(),
		KerberosTicketPaths: response.GetDeletedKerberosFilePaths(),
	}

	return credentialsFetcherResponse, nil
}
