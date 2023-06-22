/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"github.com/kubernetes-csi/csi-proxy/client/apiversion"
)

const (
	// pipePrefix is the prefix for Windows named pipes' names
	pipePrefix = `\\.\\pipe\\`

	// CsiProxyNamedPipePrefix is the prefix for the named pipes the proxy creates.
	// The suffix will be the API group and version,
	// e.g. "\\.\\pipe\\csi-proxy-iscsi-v1", "\\.\\pipe\\csi-proxy-filesystem-v2alpha1", etc.
	csiProxyNamedPipePrefix = "csi-proxy-"
)

func PipePath(apiGroupName string, apiVersion apiversion.Version) string {
	return pipePrefix + csiProxyNamedPipePrefix + apiGroupName + "-" + apiVersion.String()
}
