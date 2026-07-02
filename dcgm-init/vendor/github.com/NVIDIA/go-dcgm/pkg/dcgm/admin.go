/*
 * Copyright (c) 2020, NVIDIA CORPORATION.  All rights reserved.
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
#cgo linux LDFLAGS: -ldl -Wl,--export-dynamic -Wl,--unresolved-symbols=ignore-in-object-files
#cgo darwin LDFLAGS: -ldl -Wl,-undefined,dynamic_lookup

#include <dlfcn.h>
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

type mode int

// const for DCGM hostengine running modes: Embedded, Standalone or StartHostengine
const (
	Embedded mode = iota
	Standalone
	StartHostengine
)

type dcgmHandle struct{ handle C.dcgmHandle_t }

var (
	dcgmLibHandle        unsafe.Pointer
	stopMode             mode
	handle               dcgmHandle
	hostengineAsChildPid int
)

func initDCGM(m mode, args ...string) (err error) {
	const (
		dcgmLib = "libdcgm.so.4"
	)
	lib := C.CString(dcgmLib)
	defer freeCString(lib)

	dcgmLibHandle = C.dlopen(lib, C.RTLD_LAZY|C.RTLD_GLOBAL)
	if dcgmLibHandle == nil {
		return fmt.Errorf("%s not found", dcgmLib)
	}

	// set the stopMode for shutdown()
	stopMode = m

	switch m {
	case Embedded:
		return startEmbedded()
	case Standalone:
		return connectStandalone(args...)
	case StartHostengine:
		return startHostengine()
	default:
		panic(ErrInvalidMode)
	}
}

func shutdown() (err error) {
	switch stopMode {
	case Embedded:
		err = stopEmbedded()
	case Standalone:
		err = disconnectStandalone()
	case StartHostengine:
		err = stopHostengine()
	}

	C.dlclose(dcgmLibHandle)
	return
}

func startEmbedded() (err error) {
	result := C.dcgmInit()
	if err = errorString(result); err != nil {
		return fmt.Errorf("error initializing DCGM: %s", err)
	}

	var cHandle C.dcgmHandle_t
	result = C.dcgmStartEmbedded(C.DCGM_OPERATION_MODE_AUTO, &cHandle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error starting nv-hostengine: %s", err)
	}
	handle = dcgmHandle{cHandle}
	return
}

func stopEmbedded() (err error) {
	result := C.dcgmStopEmbedded(handle.handle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error stopping nv-hostengine: %s", err)
	}

	result = C.dcgmShutdown()
	if err = errorString(result); err != nil {
		return fmt.Errorf("error shutting down DCGM: %s", err)
	}
	return
}

type standaloneConnection struct {
	address    string
	socketFlag string
	useV3      bool
}

func standaloneConnectionArgs(args ...string) (standaloneConnection, error) {
	if len(args) > 0 && isDCGMConnectionString(args[0]) {
		return standaloneConnection{address: args[0], useV3: true}, nil
	}
	if len(args) < 2 {
		return standaloneConnection{}, errors.New("missing dcgm address and / or port")
	}
	return standaloneConnection{address: args[0], socketFlag: args[1]}, nil
}

func isDCGMConnectionString(address string) bool {
	lower := strings.ToLower(address)
	return strings.HasPrefix(lower, "tcp://") ||
		strings.HasPrefix(lower, "unix://") ||
		strings.HasPrefix(lower, "vsock://")
}

func dcgmSymbolAvailable(symbol string) bool {
	if dcgmLibHandle == nil {
		return false
	}
	cSymbol := C.CString(symbol)
	defer freeCString(cSymbol)
	return C.dlsym(dcgmLibHandle, cSymbol) != nil
}

func connectStandalone(args ...string) (err error) {
	conn, err := standaloneConnectionArgs(args...)
	if err != nil {
		return err
	}
	if conn.useV3 {
		return connectStandaloneV3(conn.address)
	}
	return connectStandaloneV2(conn.address, conn.socketFlag)
}

func connectStandaloneV2(address, socketFlag string) (err error) {
	var (
		cHandle       C.dcgmHandle_t
		connectParams C.dcgmConnectV2Params_v2
	)

	result := C.dcgmInit()
	if err = errorString(result); err != nil {
		return fmt.Errorf("error initializing DCGM: %s", err)
	}

	addr := C.CString(address)
	defer freeCString(addr)
	connectParams.version = makeVersion2(unsafe.Sizeof(connectParams))

	sck, err := strconv.ParseUint(socketFlag, 10, 32)
	if err != nil {
		return fmt.Errorf("error parsing %s: %v", socketFlag, err)
	}
	connectParams.addressIsUnixSocket = C.uint(sck)

	result = C.dcgmConnect_v2(addr, &connectParams, &cHandle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error connecting to nv-hostengine: %s", err)
	}

	handle = dcgmHandle{cHandle}
	return nil
}

func connectStandaloneV3(connectionString string) (err error) {
	const dcgmConnectV3Symbol = "dcgmConnect_v3"
	if !dcgmSymbolAvailable(dcgmConnectV3Symbol) {
		return fmt.Errorf("%s is not available in libdcgm.so.4; DCGM connection strings require DCGM 4.5.0 or newer", dcgmConnectV3Symbol)
	}

	var (
		cHandle       C.dcgmHandle_t
		connectParams C.dcgmConnectV3Params_v1
	)

	result := C.dcgmInit()
	if err = errorString(result); err != nil {
		return fmt.Errorf("error initializing DCGM: %s", err)
	}

	cConnectionString := C.CString(connectionString)
	defer freeCString(cConnectionString)
	connectParams.version = makeVersion1(unsafe.Sizeof(connectParams))

	result = C.dcgmConnect_v3(cConnectionString, &connectParams, &cHandle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error connecting to nv-hostengine: %s", err)
	}

	handle = dcgmHandle{cHandle}
	return nil
}

func disconnectStandalone() (err error) {
	result := C.dcgmDisconnect(handle.handle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error disconnecting from nv-hostengine: %s", err)
	}

	result = C.dcgmShutdown()
	if err = errorString(result); err != nil {
		return fmt.Errorf("error shutting down DCGM: %s", err)
	}
	return
}

func startHostengine() (err error) {
	var (
		procAttr      syscall.ProcAttr
		cHandle       C.dcgmHandle_t
		connectParams C.dcgmConnectV2Params_v2
	)

	bin, err := exec.LookPath("nv-hostengine")
	if err != nil {
		return fmt.Errorf("error finding nv-hostengine: %s", err)
	}
	procAttr.Files = []uintptr{
		uintptr(syscall.Stdin),
		uintptr(syscall.Stdout),
		uintptr(syscall.Stderr),
	}
	procAttr.Sys = &syscall.SysProcAttr{Setpgid: true}

	dir := "/tmp"
	tmpfile, err := os.CreateTemp(dir, "dcgm")
	if err != nil {
		return fmt.Errorf("error creating temporary file in %s directory: %s", dir, err)
	}
	socketPath := tmpfile.Name()
	defer os.Remove(socketPath)

	connectArg := "--domain-socket"
	hostengineAsChildPid, err = syscall.ForkExec(bin, []string{bin, connectArg, socketPath}, &procAttr)
	if err != nil {
		return fmt.Errorf("error fork-execing nv-hostengine: %s", err)
	}

	result := C.dcgmInit()
	if err = errorString(result); err != nil {
		return fmt.Errorf("error initializing DCGM: %s", err)
	}

	connectParams.version = makeVersion2(unsafe.Sizeof(connectParams))
	isSocket := C.uint(1)
	connectParams.addressIsUnixSocket = isSocket
	cSockPath := C.CString(socketPath)
	defer freeCString(cSockPath)
	result = C.dcgmConnect_v2(cSockPath, &connectParams, &cHandle)
	if err = errorString(result); err != nil {
		return fmt.Errorf("error connecting to nv-hostengine: %s", err)
	}

	handle = dcgmHandle{cHandle}
	return
}

// AttachDriver attaches the driver to DCGM.
// This is used to reattach the driver after a DetachDriver call, typically when updating
// the driver without restarting DCGM.
// Requires DCGM 4.5.0 or later.
func AttachDriver() error {
	result := C.dcgmAttachDriver(handle.handle)
	if result != C.DCGM_ST_OK {
		return &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}
	return nil
}

// DetachDriver detaches the driver from DCGM.
// This is used when you want to update the driver without restarting DCGM.
// After detaching, GPUs will not be accessible until AttachDriver is called.
// Requires DCGM 4.5.0 or later.
func DetachDriver() error {
	result := C.dcgmDetachDriver(handle.handle)
	if result != C.DCGM_ST_OK {
		return &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}
	return nil
}

func stopHostengine() (err error) {
	if err = disconnectStandalone(); err != nil {
		return
	}

	// terminate nv-hostengine
	cmd := exec.Command("nv-hostengine", "--term")
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("error terminating nv-hostengine: %s", err)
	}

	log.Println("Successfully terminated nv-hostengine.")

	return syscall.Kill(hostengineAsChildPid, syscall.SIGKILL)
}
