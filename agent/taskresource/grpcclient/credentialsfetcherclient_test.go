//go:build unit
// +build unit

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
package grpcclient

import (
	"context"
	"log"
	"net"
	"testing"
	"time"

	pb "github.com/aws/amazon-ecs-agent/agent/taskresource/grpcclient/credentialsfetcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	leaseid           = "123456"
	credspec_webapp01 = "{\"CmsPlugins\":[\"ActiveDirectory\"],\"DomainJoinConfig\":{\"Sid\":\"S-1-5-21-4217655605-3681839426-3493040985\",\"MachineAccountName\":\"WebApp01\",\"Guid\":\"af602f85-d754-4eea-9fa8-fd76810485f1\",\"DnsTreeName\":\"contoso.com\",\"DnsName\":\"contoso.com\",\"NetBiosName\":\"contoso\"},\"ActiveDirectoryConfig\":{\"GroupManagedServiceAccounts\":[{\"Name\":\"WebApp01\",\"Scope\":\"contoso.com\"},{\"Name\":\"WebApp01\",\"Scope\":\"contoso\"}]}}"
)

type mockCredentialsFetcherServer struct {
	pb.UnimplementedCredentialsFetcherServiceServer
}

func (*mockCredentialsFetcherServer) AddKerberosLease(ctx context.Context, req *pb.CreateKerberosLeaseRequest) (*pb.CreateKerberosLeaseResponse, error) {
	if len(req.GetCredspecContents()) == 0 {
		return &pb.CreateKerberosLeaseResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecs request should not be empty")
	}

	return &pb.CreateKerberosLeaseResponse{LeaseId: leaseid, CreatedKerberosFilePaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}}, nil
}

func (*mockCredentialsFetcherServer) AddNonDomainJoinedKerberosLease(ctx context.Context, req *pb.CreateNonDomainJoinedKerberosLeaseRequest) (*pb.CreateNonDomainJoinedKerberosLeaseResponse, error) {
	if len(req.GetCredspecContents()) == 0 {
		return &pb.CreateNonDomainJoinedKerberosLeaseResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecs request should not be empty")
	}

	if len(req.GetUsername()) == 0 || len(req.GetPassword()) == 0 || len(req.GetDomain()) == 0 {
		return &pb.CreateNonDomainJoinedKerberosLeaseResponse{}, status.Errorf(codes.InvalidArgument, "username, password or domain should not be empty")
	}

	return &pb.CreateNonDomainJoinedKerberosLeaseResponse{LeaseId: leaseid, CreatedKerberosFilePaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}}, nil
}

func (*mockCredentialsFetcherServer) RenewNonDomainJoinedKerberosLease(ctx context.Context, req *pb.RenewNonDomainJoinedKerberosLeaseRequest) (*pb.RenewNonDomainJoinedKerberosLeaseResponse, error) {
	if len(req.GetUsername()) == 0 || len(req.GetPassword()) == 0 || len(req.GetDomain()) == 0 {
		return &pb.RenewNonDomainJoinedKerberosLeaseResponse{}, status.Errorf(codes.InvalidArgument, "username, password or domain should not be empty")
	}

	return &pb.RenewNonDomainJoinedKerberosLeaseResponse{RenewedKerberosFilePaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}}, nil
}

func (*mockCredentialsFetcherServer) DeleteKerberosLease(ctx context.Context, req *pb.DeleteKerberosLeaseRequest) (*pb.DeleteKerberosLeaseResponse, error) {
	if len(req.GetLeaseId()) == 0 {
		return &pb.DeleteKerberosLeaseResponse{}, status.Errorf(codes.InvalidArgument, "credentialspecs request should not be empty")
	}

	return &pb.DeleteKerberosLeaseResponse{LeaseId: leaseid, DeletedKerberosFilePaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}}, nil
}

func dialer() func(context.Context, string) (net.Conn, error) {
	listener := bufconn.Listen(1024 * 1024)

	server := grpc.NewServer()

	pb.RegisterCredentialsFetcherServiceServer(server, &mockCredentialsFetcherServer{})

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	return func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
}

func TestCredentialsFetcherClient_AddKerberosLease(t *testing.T) {
	tests := []struct {
		name             string
		credspecContents []string
		response         CredentialsFetcherResponse
		expectedError    string
	}{
		{
			"invalid request empty credspec contents",
			[]string{},
			CredentialsFetcherResponse{},
			"rpc error: code = InvalidArgument desc = credentialspecs should not be empty",
		},
		{
			"valid request credspecs associated to gMSA account",
			[]string{credspec_webapp01},
			CredentialsFetcherResponse{LeaseID: leaseid, KerberosTicketPaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}},
			"",
		},
	}

	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "", grpc.WithInsecure(), grpc.WithContextDialer(dialer()))
	require.NoError(t, err)
	defer conn.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := NewCredentialsFetcherClient(conn, time.Minute).AddKerberosLease(context.Background(), tt.credspecContents)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.Equal(t, tt.response, response)
			}
		})
	}
}

func TestCredentialsFetcherClient_AddNonDomainJoinedKerberosLease(t *testing.T) {
	tests := []struct {
		name             string
		credspecContents []string
		username         string
		password         string
		domain           string
		response         CredentialsFetcherResponse
		expectedError    string
	}{
		{
			"invalid request empty credspec contents",
			[]string{},
			"testusername",
			"testpassword",
			"testdomain",
			CredentialsFetcherResponse{},
			"rpc error: code = InvalidArgument desc = credentialspecs should not be empty",
		},
		{
			"invalid request username, password or domain should not be empty",
			[]string{credspec_webapp01},
			"",
			"",
			"",
			CredentialsFetcherResponse{},
			"rpc error: code = InvalidArgument desc = username, password or domain should not be empty",
		},
		{
			"valid request credspecs associated to gMSA account",
			[]string{credspec_webapp01},
			"testusername",
			"testpassword",
			"testdomain",
			CredentialsFetcherResponse{LeaseID: leaseid, KerberosTicketPaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}},
			"",
		},
	}

	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "", grpc.WithInsecure(), grpc.WithContextDialer(dialer()))
	require.NoError(t, err)
	defer conn.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := NewCredentialsFetcherClient(conn, time.Minute).AddNonDomainJoinedKerberosLease(context.Background(), tt.credspecContents, tt.username, tt.password, tt.domain)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.Equal(t, tt.response, response)
			}
		})
	}
}

func TestCredentialsFetcherClient_RenewNonDomainJoinedKerberosLease(t *testing.T) {
	tests := []struct {
		name          string
		username      string
		password      string
		domain        string
		response      CredentialsFetcherResponse
		expectedError string
	}{
		{
			"invalid request username, password or domain should not be empty",
			"",
			"",
			"",
			CredentialsFetcherResponse{},
			"rpc error: code = InvalidArgument desc = username, password or domain should not be empty",
		},
		{
			"valid request credspecs associated to gMSA account",
			"testusername",
			"testpassword",
			"testdomain",
			CredentialsFetcherResponse{LeaseID: "", KerberosTicketPaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}},
			"",
		},
	}

	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "", grpc.WithInsecure(), grpc.WithContextDialer(dialer()))
	require.NoError(t, err)
	defer conn.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := NewCredentialsFetcherClient(conn, time.Minute).RenewNonDomainJoinedKerberosLease(context.Background(), tt.username, tt.password, tt.domain)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.Equal(t, tt.response, response)
			}
		})
	}
}

func TestCredentialsFetcherClient_DeleteKerberosLease(t *testing.T) {
	tests := []struct {
		name          string
		leaseid       string
		response      CredentialsFetcherResponse
		expectedError string
	}{
		{
			"invalid request empty leaseid input",
			"",
			CredentialsFetcherResponse{},
			"rpc error: code = InvalidArgument desc = invalid leaseid provided",
		},
		{
			"valid request credspecs associated to gMSA account",
			leaseid,
			CredentialsFetcherResponse{LeaseID: leaseid, KerberosTicketPaths: []string{"/var/credentials-fetcher/krbdir/123456/webapp01", "/var/credentials-fetcher/krbdir/123456/webapp02"}},
			"",
		},
	}

	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "", grpc.WithInsecure(), grpc.WithContextDialer(dialer()))
	require.NoError(t, err)
	defer conn.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := NewCredentialsFetcherClient(conn, time.Minute).DeleteKerberosLease(context.Background(), tt.leaseid)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.Equal(t, tt.response, response)
			}
		})
	}
}
