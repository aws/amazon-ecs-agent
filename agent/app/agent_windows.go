//go:build windows
// +build windows

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

package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eni/networkutils"
	"github.com/aws/amazon-ecs-agent/agent/eni/watcher"
	fsxfactory "github.com/aws/amazon-ecs-agent/agent/fsx/factory"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
)

const (
	//EcsSvcName is the name of the service
	EcsSvcName = "AmazonECS"
)

// awsVPCCNIPlugins is a list of CNI plugins required by the ECS Agent to configure the ENI for a task
var awsVPCCNIPlugins = []string{
	ecscni.ECSVPCENIPluginExecutable,
}

// initializeTaskENIDependencies initializes all the dependencies required to support awsvpc mode.
// In case of error, the returned bool is used to identify if the error is terminal.
func (agent *ecsAgent) initializeTaskENIDependencies(state dockerstate.TaskEngineState, taskEngine engine.TaskEngine) (error, bool) {

	// Set VPC and Subnet IDs for the instance
	if err, ok := agent.setVPCSubnet(); err != nil {
		return err, ok
	}

	// Validate that the CNI plugins exist in the expected path
	// We also validate that it has required flags in the capability string to support AWSVPC mode
	if err := agent.verifyCNIPluginsCapabilities(); err != nil {
		// An error here is terminal as it means that the plugins do not support the task networking capability
		return err, true
	}

	// We start the ENI Watcher which is required for awsvpc mode.
	if err := agent.startENIWatcher(state, taskEngine.StateChangeEvents()); err != nil {
		// If the ENI Watcher cannot be started then it is possible to start it on the next run.
		// Therefore, this is not a terminal error. Thus returning error and false
		return err, false
	}

	// The instance ENI and the task ENI on ECS EC2 Windows will belong to the same VPC, and therefore,
	// have the same DNS server list. Hence, we store the DNS server list of the instance ENI during
	// agent startup and use the same during config creation for setting up task ENI.
	// Another intrinsic benefit of this approach is that any DNS servers added for Active Directory
	// will be added to the task ENI, allowing tasks in awsvpc network mode to support gMSA.
	dnsServerList, err := agent.resourceFields.NetworkUtils.GetDNSServerAddressList(agent.mac)
	if err != nil {
		// An error at this point is terminal as the tasks launched with awsvpc network mode
		// require the DNS entries.
		return fmt.Errorf("unable to get dns server addresses of instance eni: %v", err), true
	}
	agent.cfg.InstanceENIDNSServerList = dnsServerList

	return nil, false
}

// startWindowsService runs the ECS agent as a Windows Service
func (agent *ecsAgent) startWindowsService() int {
	svc.Run(EcsSvcName, newHandler(agent))
	return 0
}

func (agent *ecsAgent) startEBSWatcher(state dockerstate.TaskEngineState, taskEngine engine.TaskEngine) error {
	seelog.Debug("Windows EBS Watcher not implemented: No Op")
	return nil
}

// handler implements https://godoc.org/golang.org/x/sys/windows/svc#Handler
type handler struct {
	ecsAgent agent
}

func newHandler(agent agent) *handler {
	return &handler{agent}
}

// Execute implements https://godoc.org/golang.org/x/sys/windows/svc#Handler
// The basic way that this implementation works is through two channels (representing the requests from Windows and the
// responses we're sending to Windows) and two goroutines (one for message processing with Windows and the other to
// actually run the agent).  Once we've set everything up and started both goroutines, we wait for either one to exit
// (the Windows goroutine will exit based on messages from Windows while the agent goroutine exits if the agent exits)
// and then cancel the other.  Once everything has stopped running, this function returns and the process exits.
func (h *handler) Execute(args []string, requests <-chan svc.ChangeRequest, responses chan<- svc.Status) (bool, uint32) {
	defer seelog.Flush()
	// channels for communication between goroutines
	ctx, cancel := context.WithCancel(context.Background())
	agentDone := make(chan struct{})
	windowsDone := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer close(windowsDone)
		defer wg.Done()
		h.handleWindowsRequests(ctx, requests, responses)
	}()

	var agentExitCode uint32
	go func() {
		defer close(agentDone)
		defer wg.Done()
		agentExitCode = h.runAgent(ctx)
	}()

	// Wait until one of the goroutines is either told to stop or fails spectacularly.  Under normal conditions we will
	// be waiting here for a long time.
	select {
	case <-windowsDone:
		// Service was told to stop by the Windows API.  This happens either through manual intervention (i.e.,
		// "Stop-Service AmazonECS") or through system shutdown.  Regardless, this is a normal exit event and not an
		// error.
		seelog.Info("Received normal signal from Windows to exit")
	case <-agentDone:
		// This means that the agent stopped on its own.  This is where it's appropriate to light the event log on fire
		// and set off all the alarms.
		seelog.Errorf("Exiting %d", agentExitCode)
	}
	cancel()
	wg.Wait()
	seelog.Infof("Bye Bye! Exiting with %d", agentExitCode)
	return true, agentExitCode
}

// handleWindowsRequests is a loop intended to run in a goroutine.  It handles bidirectional communication with the
// Windows service manager.  This function works by pretty much immediately moving to running and then waiting for a
// stop or shut down message from Windows or to be canceled (which could happen if the agent exits by itself and the
// calling function cancels the context).
func (h *handler) handleWindowsRequests(ctx context.Context, requests <-chan svc.ChangeRequest, responses chan<- svc.Status) {
	// Immediately tell Windows that we are pending service start.
	responses <- svc.Status{State: svc.StartPending}
	seelog.Info("Starting Windows service")

	// TODO: Pre-start hooks go here (unclear if we need any yet)

	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms682108(v=vs.85).aspx
	// Not sure if a better link exists to describe what these values mean
	accepts := svc.AcceptStop | svc.AcceptShutdown

	// Announce that we are running and we accept the above-mentioned commands
	responses <- svc.Status{State: svc.Running, Accepts: accepts}

	defer func() {
		// Announce that we are stopping
		seelog.Info("Stopping Windows service")
		responses <- svc.Status{State: svc.StopPending}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				// Our status doesn't change unless we are told to stop or shutdown
				responses <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				return
			default:
				continue
			}
		}
	}
}

// runAgent runs the ECS agent inside a goroutine and waits to be told to exit.
func (h *handler) runAgent(ctx context.Context) uint32 {
	agentCtx, cancel := context.WithCancel(ctx)
	indicator := newTermHandlerIndicator()

	terminationHandler := func(state dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc) {
		// We're using a custom indicator to record that the handler is scheduled to be executed (has been invoked) and
		// to determine whether it should run (we skip when the agent engine has already exited).  After recording to
		// the indicator that the handler has been invoked, we wait on the context.  When we wake up, we determine
		// whether to execute or not based on whether the agent is still running.
		defer indicator.finish()
		indicator.setInvoked()
		<-agentCtx.Done()
		if !indicator.isAgentRunning() {
			return
		}

		seelog.Info("Termination handler received signal to stop")
		err := sighandlers.FinalSave(state, dataClient, taskEngine)
		if err != nil {
			seelog.Criticalf("Error saving state before final shutdown: %v", err)
		}
	}
	h.ecsAgent.setTerminationHandler(terminationHandler)

	go func() {
		defer cancel()
		exitCode := h.ecsAgent.start() // should block forever, unless there is an error

		if exitCode == exitcodes.ExitTerminal {
			seelog.Critical("Terminal exit code received from agent.  Windows SCM will not restart the AmazonECS service.")
			// We override the exit code to 0 here so that Windows does not treat this as a restartable failure even
			// when "sc.exe failureflag" is set.
			exitCode = 0
		}

		indicator.agentStopped(exitCode)
	}()

	sleepCtx(agentCtx, time.Minute) // give the agent a minute to start and invoke terminationHandler

	// wait for the termination handler to run.  Once the termination handler runs, we can safely exit.  If the agent
	// exits by itself, the termination handler doesn't need to do anything and skips.  If the agent exits before the
	// termination handler is invoked, we can exit immediately.
	return indicator.wait()
}

// sleepCtx provides a cancelable sleep
func sleepCtx(ctx context.Context, duration time.Duration) {
	derivedCtx, _ := context.WithDeadline(ctx, time.Now().Add(duration))
	<-derivedCtx.Done()
}

type termHandlerIndicator struct {
	mu             sync.Mutex
	agentRunning   bool
	exitCode       uint32
	handlerInvoked bool
	handlerDone    chan struct{}
}

func newTermHandlerIndicator() *termHandlerIndicator {
	return &termHandlerIndicator{
		agentRunning:   true,
		handlerInvoked: false,
		handlerDone:    make(chan struct{}),
	}
}

func (t *termHandlerIndicator) isAgentRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.agentRunning
}

func (t *termHandlerIndicator) agentStopped(exitCode int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agentRunning = false
	t.exitCode = uint32(exitCode)
}

func (t *termHandlerIndicator) finish() {
	close(t.handlerDone)
}

func (t *termHandlerIndicator) setInvoked() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handlerInvoked = true
}

func (t *termHandlerIndicator) wait() uint32 {
	t.mu.Lock()
	invoked := t.handlerInvoked
	t.mu.Unlock()
	if invoked {
		<-t.handlerDone
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitCode
}

func (agent *ecsAgent) initializeResourceFields(credentialsManager credentials.Manager) {
	agent.resourceFields = &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			ASMClientCreator:   asmfactory.NewClientCreator(),
			SSMClientCreator:   ssmfactory.NewSSMClientCreator(),
			FSxClientCreator:   fsxfactory.NewFSxClientCreator(),
			S3ClientCreator:    s3factory.NewS3ClientCreator(),
			CredentialsManager: credentialsManager,
		},
		Ctx:          agent.ctx,
		DockerClient: agent.dockerClient,
		NetworkUtils: networkutils.New(),
	}
}

func (agent *ecsAgent) cgroupInit() error {
	return errors.New("unsupported platform")
}

func (agent *ecsAgent) initializeGPUManager() error {
	return nil
}

func (agent *ecsAgent) getPlatformDevices() []*ecs.PlatformDevice {
	return nil
}

func (agent *ecsAgent) loadPauseContainer() error {
	// The pause image would be cached in th ECS-Optimized Windows AMI's and will be available. We will throw an error if the image is not loaded.
	// If the agent is run on non-supported instances then pause image has to be loaded manually by the client.
	_, err := agent.pauseLoader.IsLoaded(agent.dockerClient)

	return err
}

// This method starts the eni watcher
func (agent *ecsAgent) startENIWatcher(state dockerstate.TaskEngineState, stateChangeEvents chan<- statechange.Event) error {
	seelog.Debug("Starting ENI Watcher")
	eniWatcher, err := watcher.New(agent.ctx, agent.mac, state, stateChangeEvents)
	if err != nil {
		return errors.Wrapf(err, "unable to start eni watcher")
	}
	if err := eniWatcher.Init(); err != nil {
		return errors.Wrapf(err, "unable to start eni watcher")
	}
	go eniWatcher.Start()
	return nil
}

// This function verifies that the plugins are available at the given path
// When each plugin is executed with --capabilities then it returns a string representing its capabilities
// Using this we will verify that CNI Plugin can support AWSVPC mode
func (agent *ecsAgent) verifyCNIPluginsCapabilities() error {
	for _, plugin := range awsVPCCNIPlugins {
		capabilities, err := agent.cniClient.Capabilities(plugin)
		if err != nil {
			return err
		}

		if !contains(capabilities, ecscni.CapabilityAWSVPCNetworkingMode) {
			return errors.Errorf("plugin '%s' doesn't support the capability: %s",
				plugin, ecscni.CapabilityAWSVPCNetworkingMode)
		}
	}
	return nil
}
