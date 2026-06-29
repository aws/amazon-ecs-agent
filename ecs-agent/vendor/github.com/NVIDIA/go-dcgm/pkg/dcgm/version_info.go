/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"unsafe"
)

// VersionInfo holds DCGM build environment information.
// RawBuildInfoString contains key-value pairs (e.g. version, arch, buildid, commit)
// separated by semicolons; each pair is "key:value".
type VersionInfo struct {
	RawBuildInfoString string
}

func versionInfo() (VersionInfo, error) {
	var cVersionInfo C.dcgmVersionInfo_t
	cVersionInfo.version = makeVersion2(unsafe.Sizeof(cVersionInfo))

	result := C.dcgmVersionInfo(&cVersionInfo)
	if err := errorString(result); err != nil {
		return VersionInfo{}, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	return VersionInfo{
		RawBuildInfoString: C.GoString(&cVersionInfo.rawBuildInfoString[0]),
	}, nil
}

func hostengineVersionInfo() (VersionInfo, error) {
	var cVersionInfo C.dcgmVersionInfo_t
	cVersionInfo.version = makeVersion2(unsafe.Sizeof(cVersionInfo))

	result := C.dcgmHostengineVersionInfo(handle.handle, &cVersionInfo)
	if err := errorString(result); err != nil {
		return VersionInfo{}, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	return VersionInfo{
		RawBuildInfoString: C.GoString(&cVersionInfo.rawBuildInfoString[0]),
	}, nil
}
