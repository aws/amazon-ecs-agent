package gmsacredclient

import (
	"context"
	"os"
	"time"

	pb "github.com/aws/amazon-ecs-agent/ecs-agent/gmsacredclient/credentialsfetcher"
	"github.com/cihub/seelog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type CredentialsFetcherClient struct {
	conn    *grpc.ClientConn
	timeout time.Duration
}

// GetGrpcClientConnection() returns grpc client connection
func GetGrpcClientConnection() (*grpc.ClientConn, error) {
	address, err := getSocketAddress()
	if err != nil {
		seelog.Errorf("could not find path to credentials fetcher host dir : %v", err)
		return nil, err
	}

	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		seelog.Errorf("could not initialize client connection %v", err)
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

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	response, err := client.HealthCheck(ctx, request)
	if err != nil {
		seelog.Errorf("credentials-fetcher daemon status is unhealthy during health check: %v", err)
		return "", err
	}
	seelog.Debugf("credentials-fetcher daemon is running")

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

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	response, err := client.AddKerberosArnLease(ctx, request)
	if err != nil {
		seelog.Errorf("could not create kerberos tickets: %v", err)
		return CredentialsFetcherArnResponse{}, err
	}
	seelog.Infof("created kerberos tickets and associated with LeaseID: %s", response.GetLeaseId())

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
func (c CredentialsFetcherClient) RenewKerberosArnLease(ctx context.Context, credentialspecsArns []string, accessKeyId string, secretKey string, sessionToken string, region string) (string, error) {
	if len(credentialspecsArns) == 0 {
		return codes.InvalidArgument.String(), status.Errorf(codes.InvalidArgument, "credentialspecArns should not be empty")
	}

	if len(accessKeyId) == 0 || len(secretKey) == 0 || len(sessionToken) == 0 || len(region) == 0 {
		return codes.InvalidArgument.String(), status.Errorf(codes.InvalidArgument, "accessid, secretkey, sessiontoken or region should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.KerberosArnLeaseRequest{CredspecArns: credentialspecsArns, AccessKeyId: accessKeyId, SecretAccessKey: secretKey, SessionToken: sessionToken, Region: region}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	response, err := client.RenewKerberosArnLease(ctx, request)
	if err != nil {
		seelog.Errorf("could not renew kerberos tickets: %v", err)
		return codes.Internal.String(), err
	}

	if response.GetStatus() == "failed" {
		return codes.Internal.String(), status.Errorf(codes.Internal, "renewal of kerberos tickets failed")
	}

	seelog.Infof("renewal of kerberos tickets are successful")

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

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	response, err := client.AddKerberosLease(ctx, request)
	if err != nil {
		seelog.Errorf("could not create kerberos tickets: %v", err)
		return CredentialsFetcherResponse{}, err
	}
	seelog.Infof("created kerberos tickets and associated with LeaseID: %s", response.GetLeaseId())

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
		seelog.Error("credentialspecs request should not be empty")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecs should not be empty")
	}

	if len(username) == 0 || len(password) == 0 || len(domain) == 0 {
		seelog.Error("username, password or domain should not be empty")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "username, password or domain should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.CreateNonDomainJoinedKerberosLeaseRequest{CredspecContents: credentialspecs, Username: username, Password: password, Domain: domain}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	response, err := client.AddNonDomainJoinedKerberosLease(ctx, request)
	if err != nil {
		seelog.Errorf("could not create kerberos tickets: %v", err)
		return CredentialsFetcherResponse{}, err
	}
	seelog.Infof("created kerberos tickets and associated with LeaseID: %s", response.GetLeaseId())

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
		seelog.Error("username, password or domain should not be empty")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "username, password or domain should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.RenewNonDomainJoinedKerberosLeaseRequest{Username: username, Password: password, Domain: domain}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	response, err := client.RenewNonDomainJoinedKerberosLease(ctx, request)
	if err != nil {
		seelog.Errorf("could not renew kerberos tickets: %v", err)
		return CredentialsFetcherResponse{}, err
	}

	credentialsFetcherResponse := CredentialsFetcherResponse{
		KerberosTicketPaths: response.GetRenewedKerberosFilePaths(),
	}

	return credentialsFetcherResponse, nil
}

// DeleteKerberosLease() invokes credentials fetcher daemon running on the host
// to delete kerberos tickets of gMSA accounts associated with the leaseid
func (c CredentialsFetcherClient) DeleteKerberosLease(ctx context.Context, leaseid string) (CredentialsFetcherResponse, error) {
	if len(leaseid) == 0 {
		seelog.Error("invalid leaseid provided")
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "invalid leaseid provided")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.DeleteKerberosLeaseRequest{LeaseId: leaseid}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	response, err := client.DeleteKerberosLease(ctx, request)
	if err != nil {
		seelog.Errorf("could not delete kerberos tickets: %v", err)
		return CredentialsFetcherResponse{}, err
	}
	seelog.Infof("deleted kerberos associated with LeaseID: %s", response.GetLeaseId())

	credentialsFetcherResponse := CredentialsFetcherResponse{
		LeaseID:             response.GetLeaseId(),
		KerberosTicketPaths: response.GetDeletedKerberosFilePaths(),
	}

	return credentialsFetcherResponse, nil
}
