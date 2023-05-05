package grpcclient

import (
	"context"
	"os"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/aws/amazon-ecs-agent/agent/taskresource/grpcclient/credentialsfetcher"
	"github.com/cihub/seelog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

// AddKerberosLease() invokes credentials fetcher daemon running on the host
// to create kerberos tickets associated with gMSA accounts
func (c CredentialsFetcherClient) AddKerberosLease(ctx context.Context, credentialspecs []string) (CredentialsFetcherResponse, error) {
	if len(credentialspecs) == 0 {
		return CredentialsFetcherResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecs should not be empty")
	}

	defer c.conn.Close()
	client := pb.NewCredentialsFetcherServiceClient(c.conn)

	request := &pb.CreateKerberosLeaseRequest{CredspecContents: credentialspecs}

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(c.timeout))
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

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(c.timeout))
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

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(c.timeout))
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

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(c.timeout))
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
