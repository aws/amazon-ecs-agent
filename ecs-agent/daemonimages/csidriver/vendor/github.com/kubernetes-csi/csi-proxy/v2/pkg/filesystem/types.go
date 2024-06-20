package filesystem

type PathExistsRequest struct {
	// The path whose existence we want to check in the host's filesystem
	Path string
}

type PathExistsResponse struct {
	// Indicates whether the path in PathExistsRequest exists in the host's filesystem
	Exists bool
}

type PathValidRequest struct {
	// The path whose validity we want to check in the host's filesystem
	Path string
}

type PathValidResponse struct {
	// Indicates whether the path in PathValidRequest is a valid path
	Valid bool
}

type MkdirRequest struct {
	// The path to create in the host's filesystem.
	// All special characters allowed by Windows in path names will be allowed
	// except for restrictions noted below. For details, please check:
	// https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file
	// Non-existent parent directories in the path will be automatically created.
	// Directories will be created with Read and Write privileges of the Windows
	// User account under which csi-proxy is started (typically LocalSystem).
	//
	// Restrictions:
	// Only absolute path (indicated by a drive letter prefix: e.g. "C:\") is accepted.
	// Depending on the context parameter of this function, the path prefix needs
	// to match the paths specified either as kubelet-csi-plugins-path
	// or as kubelet-pod-path parameters of csi-proxy.
	// The path parameter cannot already exist on host filesystem.
	// UNC paths of the form "\\server\share\path\file" are not allowed.
	// All directory separators need to be backslash character: "\".
	// Characters: .. / : | ? * in the path are not allowed.
	// Maximum path length will be capped to 260 characters.
	Path string
}

type MkdirResponse struct {
	// Intentionally empty
}

type RmdirRequest struct {
	// The path to remove in the host's filesystem.
	// All special characters allowed by Windows in path names will be allowed
	// except for restrictions noted below. For details, please check:
	// https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file
	//
	// Restrictions:
	// Only absolute path (indicated by a drive letter prefix: e.g. "C:\") is accepted.
	// Depending on the context parameter of this function, the path prefix needs
	// to match the paths specified either as kubelet-csi-plugins-path
	// or as kubelet-pod-path parameters of csi-proxy.
	// UNC paths of the form "\\server\share\path\file" are not allowed.
	// All directory separators need to be backslash character: "\".
	// Characters: .. / : | ? * in the path are not allowed.
	// Path cannot be a file of type symlink.
	// Maximum path length will be capped to 260 characters.
	Path string

	// Force remove all contents under path (if any).
	Force bool
}

type RmdirResponse struct {
	// Intentionally empty
}

type RmdirContentsRequest struct {
	// The path to remove in the host's filesystem.
	// All special characters allowed by Windows in path names will be allowed
	// except for restrictions noted below. For details, please check:
	// https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file
	//
	// Restrictions:
	// Only absolute path (indicated by a drive letter prefix: e.g. "C:\") is accepted.
	// Depending on the context parameter of this function, the path prefix needs
	// to match the paths specified either as kubelet-csi-plugins-path
	// or as kubelet-pod-path parameters of csi-proxy.
	// UNC paths of the form "\\server\share\path\file" are not allowed.
	// All directory separators need to be backslash character: "\".
	// Characters: .. / : | ? * in the path are not allowed.
	// Path cannot be a file of type symlink.
	// Maximum path length will be capped to 260 characters.
	Path string
}

type RmdirContentsResponse struct {
	// Intentionally empty
}

type CreateSymlinkRequest struct {
	// The path of the existing directory to be linked.
	// All special characters allowed by Windows in path names will be allowed
	// except for restrictions noted below. For details, please check:
	// https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file
	//
	// Restrictions:
	// Only absolute path (indicated by a drive letter prefix: e.g. "C:\") is accepted.
	// The path prefix needs needs to match the paths specified as
	// kubelet-csi-plugins-path parameter of csi-proxy.
	// UNC paths of the form "\\server\share\path\file" are not allowed.
	// All directory separators need to be backslash character: "\".
	// Characters: .. / : | ? * in the path are not allowed.
	// source_path cannot already exist in the host filesystem.
	// Maximum path length will be capped to 260 characters.
	SourcePath string

	// Target path is the location of the new directory entry to be created in the host's filesystem.
	// All special characters allowed by Windows in path names will be allowed
	// except for restrictions noted below. For details, please check:
	// https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file
	//
	// Restrictions:
	// Only absolute path (indicated by a drive letter prefix: e.g. "C:\") is accepted.
	// The path prefix needs to match the paths specified as
	// kubelet-pod-path parameter of csi-proxy.
	// UNC paths of the form "\\server\share\path\file" are not allowed.
	// All directory separators need to be backslash character: "\".
	// Characters: .. / : | ? * in the path are not allowed.
	// target_path needs to exist as a directory in the host that is empty.
	// target_path cannot be a symbolic link.
	// Maximum path length will be capped to 260 characters.
	TargetPath string
}

type CreateSymlinkResponse struct {
	// Intentionally empty
}

type IsSymlinkRequest struct {
	// The path whose existence as a symlink we want to check in the host's filesystem
	Path string
}

type IsSymlinkResponse struct {
	// Indicates whether the path in IsSymlinkRequest is a symlink
	IsSymlink bool
}
