package filesystem

import (
	"context"

	filesystemapi "github.com/kubernetes-csi/csi-proxy/v2/pkg/filesystem/hostapi"
	"k8s.io/klog/v2"
)

type Filesystem struct {
	hostAPI filesystemapi.HostAPI
}

type Interface interface {
	// CreateSymlink creates a symbolic link called target_path that points to source_path
	// in the host filesystem (target_path is the name of the symbolic link created,
	// source_path is the existing path).
	CreateSymlink(context.Context, *CreateSymlinkRequest) (*CreateSymlinkResponse, error)

	// IsSymlink checks if a given path is a symlink.
	IsSymlink(context.Context, *IsSymlinkRequest) (*IsSymlinkResponse, error)

	// Mkdir creates a directory at the requested path in the host filesystem.
	Mkdir(context.Context, *MkdirRequest) (*MkdirResponse, error)

	// PathExists checks if the requested path exists in the host filesystem.
	PathExists(context.Context, *PathExistsRequest) (*PathExistsResponse, error)

	// PathValid checks if the given path is accessible.
	PathValid(context.Context, *PathValidRequest) (*PathValidResponse, error)

	// Rmdir removes the directory at the requested path in the host filesystem.
	// This may be used for unlinking a symlink created through CreateSymlink.
	Rmdir(context.Context, *RmdirRequest) (*RmdirResponse, error)

	// RmdirContents removes the contents of a directory in the host filesystem.
	// Unlike Rmdir it won't delete the requested path, it'll only delete its contents.
	RmdirContents(context.Context, *RmdirContentsRequest) (*RmdirContentsResponse, error)
}

// check that Filesystem implements Interface
var _ Interface = &Filesystem{}

func New(hostAPI filesystemapi.HostAPI) (*Filesystem, error) {
	return &Filesystem{
		hostAPI: hostAPI,
	}, nil
}

// PathExists checks if the given path exists on the host.
func (f *Filesystem) PathExists(ctx context.Context, request *PathExistsRequest) (*PathExistsResponse, error) {
	klog.V(2).Infof("Request: PathExists with path=%q", request.Path)
	err := ValidatePathWindows(request.Path)
	if err != nil {
		klog.Errorf("failed validatePathWindows %v", err)
		return nil, err
	}
	exists, err := f.hostAPI.PathExists(request.Path)
	if err != nil {
		klog.Errorf("failed check PathExists %v", err)
		return nil, err
	}
	return &PathExistsResponse{
		Exists: exists,
	}, err
}

func (f *Filesystem) PathValid(ctx context.Context, request *PathValidRequest) (*PathValidResponse, error) {
	klog.V(2).Infof("Request: PathValid with path %q", request.Path)
	valid, err := f.hostAPI.PathValid(request.Path)
	return &PathValidResponse{
		Valid: valid,
	}, err
}

func (f *Filesystem) Mkdir(ctx context.Context, request *MkdirRequest) (*MkdirResponse, error) {
	klog.V(2).Infof("Request: Mkdir with path=%q", request.Path)
	err := ValidatePathWindows(request.Path)
	if err != nil {
		klog.Errorf("failed validatePathWindows %v", err)
		return nil, err
	}
	err = f.hostAPI.Mkdir(request.Path)
	if err != nil {
		klog.Errorf("failed Mkdir %v", err)
		return nil, err
	}

	return &MkdirResponse{}, err
}

func (f *Filesystem) Rmdir(ctx context.Context, request *RmdirRequest) (*RmdirResponse, error) {
	klog.V(2).Infof("Request: Rmdir with path=%q", request.Path)
	err := ValidatePathWindows(request.Path)
	if err != nil {
		klog.Errorf("failed validatePathWindows %v", err)
		return nil, err
	}
	err = f.hostAPI.Rmdir(request.Path, request.Force)
	if err != nil {
		klog.Errorf("failed Rmdir %v", err)
		return nil, err
	}
	return nil, err
}

func (f *Filesystem) RmdirContents(ctx context.Context, request *RmdirContentsRequest) (*RmdirContentsResponse, error) {
	klog.V(2).Infof("Request: RmdirContents with path=%q", request.Path)
	err := ValidatePathWindows(request.Path)
	if err != nil {
		klog.Errorf("failed validatePathWindows %v", err)
		return nil, err
	}
	err = f.hostAPI.RmdirContents(request.Path)
	if err != nil {
		klog.Errorf("failed RmdirContents %v", err)
		return nil, err
	}
	return nil, err
}

func (f *Filesystem) CreateSymlink(ctx context.Context, request *CreateSymlinkRequest) (*CreateSymlinkResponse, error) {
	klog.V(2).Infof("Request: CreateSymlink with targetPath=%q sourcePath=%q", request.TargetPath, request.SourcePath)
	err := ValidatePathWindows(request.TargetPath)
	if err != nil {
		klog.Errorf("failed validatePathWindows for target path %v", err)
		return nil, err
	}
	err = ValidatePathWindows(request.SourcePath)
	if err != nil {
		klog.Errorf("failed validatePathWindows for source path %v", err)
		return nil, err
	}
	err = f.hostAPI.CreateSymlink(request.SourcePath, request.TargetPath)
	if err != nil {
		klog.Errorf("failed CreateSymlink: %v", err)
		return nil, err
	}
	return &CreateSymlinkResponse{}, nil
}

func (f *Filesystem) IsSymlink(ctx context.Context, request *IsSymlinkRequest) (*IsSymlinkResponse, error) {
	klog.V(2).Infof("Request: IsSymlink with path=%q", request.Path)
	isSymlink, err := f.hostAPI.IsSymlink(request.Path)
	if err != nil {
		klog.Errorf("failed IsSymlink %v", err)
		return nil, err
	}
	return &IsSymlinkResponse{
		IsSymlink: isSymlink,
	}, nil
}
