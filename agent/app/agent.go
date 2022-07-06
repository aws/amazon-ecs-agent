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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	"github.com/aws/amazon-ecs-agent/agent/doctor"
	"github.com/aws/amazon-ecs-agent/agent/eni/watcher"
	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/amazon-ecs-agent/agent/credentials/instancecreds"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	"github.com/aws/amazon-ecs-agent/agent/metrics"

	acshandler "github.com/aws/amazon-ecs-agent/agent/acs/handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/ecsclient"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/app/factory"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eni/pause"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	tcshandler "github.com/aws/amazon-ecs-agent/agent/tcs/handler"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/mobypkgwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/aws-sdk-go/aws"
	aws_credentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/cihub/seelog"
	"github.com/pborman/uuid"
)

const (
	containerChangeEventStreamName             = "ContainerChange"
	deregisterContainerInstanceEventStreamName = "DeregisterContainerInstance"
	clusterMismatchErrorFormat                 = "Data mismatch; saved cluster '%v' does not match configured cluster '%v'. Perhaps you want to delete the configured checkpoint file?"
	instanceIDMismatchErrorFormat              = "Data mismatch; saved InstanceID '%s' does not match current InstanceID '%s'. Overwriting old datafile"
	instanceTypeMismatchErrorFormat            = "The current instance type does not match the registered instance type. Please revert the instance type change, or alternatively launch a new instance: %v"
	customAttributeErrorMessage                = " Please make sure custom attributes are valid as per public AWS documentation: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-placement-constraints.html#attributes"

	vpcIDAttributeName    = "ecs.vpc-id"
	subnetIDAttributeName = "ecs.subnet-id"

	blackholed = "blackholed"

	instanceIdBackoffMin      = time.Second
	instanceIdBackoffMax      = time.Second * 5
	instanceIdBackoffJitter   = 0.2
	instanceIdBackoffMultiple = 1.3
	instanceIdMaxRetryCount   = 3

	targetLifecycleBackoffMin      = time.Second
	targetLifecycleBackoffMax      = time.Second * 5
	targetLifecycleBackoffJitter   = 0.2
	targetLifecycleBackoffMultiple = 1.3
	targetLifecycleMaxRetryCount   = 3
	inServiceState                 = "InService"
	asgLifecyclePollWait           = time.Minute
	asgLifecyclePollMax            = 120 // given each poll cycle waits for about a minute, this gives 2-3 hours before timing out
)

var (
	instanceNotLaunchedInVPCError = errors.New("instance not launched in VPC")
)

// agent interface is used by the app runner to interact with the ecsAgent
// object. Its purpose is to mostly demonstrate how to interact with the
// ecsAgent type.
type agent interface {
	// printECSAttributes prints the Agent's capabilities based on
	// its environment
	printECSAttributes() int
	// startWindowsService starts the agent as a Windows Service
	startWindowsService() int
	// start starts the Agent execution
	start() int
	// setTerminationHandler sets the termination handler
	setTerminationHandler(sighandlers.TerminationHandler)
	// getConfig gets the agent configuration
	getConfig() *config.Config
}

// ecsAgent wraps all the entities needed to start the ECS Agent execution.
// after creating it via
// the newAgent() method
type ecsAgent struct {
	ctx                         context.Context
	cancel                      context.CancelFunc
	ec2MetadataClient           ec2.EC2MetadataClient
	ec2Client                   ec2.Client
	cfg                         *config.Config
	dataClient                  data.Client
	dockerClient                dockerapi.DockerClient
	containerInstanceARN        string
	credentialProvider          *aws_credentials.Credentials
	stateManagerFactory         factory.StateManager
	saveableOptionFactory       factory.SaveableOption
	pauseLoader                 pause.Loader
	eniWatcher                  *watcher.ENIWatcher
	cniClient                   ecscni.CNIClient
	vpc                         string
	subnet                      string
	mac                         string
	metadataManager             containermetadata.Manager
	terminationHandler          sighandlers.TerminationHandler
	mobyPlugins                 mobypkgwrapper.Plugins
	resourceFields              *taskresource.ResourceFields
	availabilityZone            string
	latestSeqNumberTaskManifest *int64
}

// newAgent returns a new ecsAgent object, but does not start anything
func newAgent(blackholeEC2Metadata bool, acceptInsecureCert *bool) (agent, error) {
	ctx, cancel := context.WithCancel(context.Background())
	ec2MetadataClient := ec2.NewEC2MetadataClient(nil)
	if blackholeEC2Metadata {
		ec2MetadataClient = ec2.NewBlackholeEC2MetadataClient()
	}
	logger.Info("Starting Amazon ECS Agent", logger.Fields{
		"version": version.Version,
		"commit":  version.GitShortHash,
	})
	logger.Info("Loading configuration")
	cfg, err := config.NewConfig(ec2MetadataClient)
	if err != nil {
		// All required config values can be inferred from EC2 Metadata,
		// so this error could be transient.
		seelog.Criticalf("Error loading config: %v", err)
		cancel()
		return nil, err
	}
	cfg.AcceptInsecureCert = aws.BoolValue(acceptInsecureCert)
	if cfg.AcceptInsecureCert {
		seelog.Warn("SSL certificate verification disabled. This is not recommended.")
	}
	seelog.Debugf("Loaded config: %s", cfg.String())

	if cfg.External.Enabled() {
		logger.Info("ECS Agent is running in external mode.")
		ec2MetadataClient = ec2.NewBlackholeEC2MetadataClient()
		cfg.NoIID = true
	}

	ec2Client := ec2.NewClientImpl(cfg.AWSRegion)
	dockerClient, err := dockerapi.NewDockerGoClient(sdkclientfactory.NewFactory(ctx, cfg.DockerEndpoint), cfg, ctx)

	if err != nil {
		// This is also non terminal in the current config
		logger.Critical("Error creating Docker client", logger.Fields{
			field.Error: err,
		})
		cancel()
		return nil, err
	}

	var dataClient data.Client
	if cfg.Checkpoint.Enabled() {
		dataClient, err = data.New(cfg.DataDir)
		if err != nil {
			logger.Critical("Error creating Docker client", logger.Fields{
				field.Error: err,
			})
			cancel()
			return nil, err
		}
	} else {
		dataClient = data.NewNoopClient()
	}

	var metadataManager containermetadata.Manager
	if cfg.ContainerMetadataEnabled.Enabled() {
		// We use the default API client for the metadata inspect call. This version has some information
		// missing which means if we need those fields later we will need to change this client to
		// the appropriate version
		metadataManager = containermetadata.NewManager(dockerClient, cfg)
	}

	initialSeqNumber := int64(-1)
	return &ecsAgent{
		ctx:               ctx,
		cancel:            cancel,
		ec2MetadataClient: ec2MetadataClient,
		ec2Client:         ec2Client,
		cfg:               cfg,
		dockerClient:      dockerClient,
		dataClient:        dataClient,
		// We instantiate our own credentialProvider for use in acs/tcs. This tries
		// to mimic roughly the way it's instantiated by the SDK for a default
		// session.
		credentialProvider:          instancecreds.GetCredentials(cfg.External.Enabled()),
		stateManagerFactory:         factory.NewStateManager(),
		saveableOptionFactory:       factory.NewSaveableOption(),
		pauseLoader:                 pause.New(),
		cniClient:                   ecscni.NewClient(cfg.CNIPluginsPath),
		metadataManager:             metadataManager,
		terminationHandler:          sighandlers.StartDefaultTerminationHandler,
		mobyPlugins:                 mobypkgwrapper.NewPlugins(),
		latestSeqNumberTaskManifest: &initialSeqNumber,
	}, nil
}

func (agent *ecsAgent) getConfig() *config.Config {
	return agent.cfg
}

// printECSAttributes prints the Agent's ECS Attributes based on its
// environment
func (agent *ecsAgent) printECSAttributes() int {
	capabilities, err := agent.capabilities()
	if err != nil {
		seelog.Warnf("Unable to obtain capabilities: %v", err)
		return exitcodes.ExitError
	}
	for _, attr := range capabilities {
		fmt.Printf("%s\t%s\n", aws.StringValue(attr.Name), aws.StringValue(attr.Value))
	}
	return exitcodes.ExitSuccess
}

func (agent *ecsAgent) setTerminationHandler(handler sighandlers.TerminationHandler) {
	agent.terminationHandler = handler
}

// start starts the ECS Agent
func (agent *ecsAgent) start() int {
	sighandlers.StartDebugHandler()

	containerChangeEventStream := eventstream.NewEventStream(containerChangeEventStreamName, agent.ctx)
	credentialsManager := credentials.NewManager()
	state := dockerstate.NewTaskEngineState()
	imageManager := engine.NewImageManager(agent.cfg, agent.dockerClient, state)
	client := ecsclient.NewECSClient(agent.credentialProvider, agent.cfg, agent.ec2MetadataClient)

	agent.initializeResourceFields(credentialsManager)
	return agent.doStart(containerChangeEventStream, credentialsManager, state, imageManager, client, execcmd.NewManager())
}

// doStart is the worker invoked by start for starting the ECS Agent. This involves
// initializing the docker task engine, state saver, image manager, credentials
// manager, poll and telemetry sessions, api handler etc
func (agent *ecsAgent) doStart(containerChangeEventStream *eventstream.EventStream,
	credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	imageManager engine.ImageManager,
	client api.ECSClient,
	execCmdMgr execcmd.Manager) int {
	// check docker version >= 1.9.0, exit agent if older
	if exitcode, ok := agent.verifyRequiredDockerVersion(); !ok {
		return exitcode
	}

	// Conditionally create '/ecs' cgroup root
	if agent.cfg.TaskCPUMemLimit.Enabled() {
		if err := agent.cgroupInit(); err != nil {
			seelog.Criticalf("Unable to initialize cgroup root for ECS: %v", err)
			return exitcodes.ExitTerminal
		}
	}
	if agent.cfg.GPUSupportEnabled {
		err := agent.initializeGPUManager()
		if err != nil {
			seelog.Criticalf("Could not initialize Nvidia GPU Manager: %v", err)
			return exitcodes.ExitError
		}
	}

	// Create the task engine
	taskEngine, currentEC2InstanceID, err := agent.newTaskEngine(containerChangeEventStream,
		credentialsManager, state, imageManager, execCmdMgr)
	if err != nil {
		seelog.Criticalf("Unable to initialize new task engine: %v", err)
		return exitcodes.ExitTerminal
	}

	// Start termination handler in goroutine
	go agent.terminationHandler(state, agent.dataClient, taskEngine, agent.cancel)

	// If part of ASG, wait until instance is being set up to go in service before registering with cluster
	if agent.cfg.WarmPoolsSupport.Enabled() {
		err := agent.waitUntilInstanceInService(asgLifecyclePollWait, asgLifecyclePollMax, targetLifecycleMaxRetryCount)
		if err != nil && err.Error() != blackholed {
			seelog.Criticalf("Could not determine target lifecycle of instance: %v", err)
			return exitcodes.ExitTerminal
		}
	}

	agent.initMetricsEngine()

	loadPauseErr := agent.loadPauseContainer()
	if loadPauseErr != nil {
		seelog.Errorf("Failed to load pause container: %v", loadPauseErr)
	}

	var vpcSubnetAttributes []*ecs.Attribute
	// Check if Task ENI is enabled
	if agent.cfg.TaskENIEnabled.Enabled() {
		// check pause container image load
		if loadPauseErr != nil {
			if pause.IsNoSuchFileError(loadPauseErr) || pause.UnsupportedPlatform(loadPauseErr) {
				return exitcodes.ExitTerminal
			} else {
				return exitcodes.ExitError
			}
		}

		err, terminal := agent.initializeTaskENIDependencies(state, taskEngine)
		switch err {
		case nil:
			// No error, we can proceed with the rest of initialization
			// Set vpc and subnet id attributes
			vpcSubnetAttributes = agent.constructVPCSubnetAttributes()
		case instanceNotLaunchedInVPCError:
			// We have ascertained that the EC2 Instance is not running in a VPC
			// No need to stop the ECS Agent in this case; all we need to do is
			// to not update the config to disable the TaskENIEnabled flag and
			// move on
			seelog.Warnf("Unable to detect VPC ID for the Instance, disabling Task ENI capability: %v", err)
			agent.cfg.TaskENIEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyDisabled}
		default:
			// Encountered an error initializing dependencies for dealing with
			// ENIs for Tasks. Exit with the appropriate error code
			seelog.Criticalf("Unable to initialize Task ENI dependencies: %v", err)
			if terminal {
				return exitcodes.ExitTerminal
			}
			return exitcodes.ExitError
		}
	}

	// Register the container instance
	err = agent.registerContainerInstance(client, vpcSubnetAttributes)
	if err != nil {
		if isTransient(err) {
			return exitcodes.ExitError
		}
		return exitcodes.ExitTerminal
	}

	// Add container instance ARN to metadata manager
	if agent.cfg.ContainerMetadataEnabled.Enabled() {
		agent.metadataManager.SetContainerInstanceARN(agent.containerInstanceARN)
		agent.metadataManager.SetAvailabilityZone(agent.availabilityZone)
		agent.metadataManager.SetHostPrivateIPv4Address(agent.getHostPrivateIPv4AddressFromEC2Metadata())
		agent.metadataManager.SetHostPublicIPv4Address(agent.getHostPublicIPv4AddressFromEC2Metadata())
	}

	if agent.cfg.Checkpoint.Enabled() {
		agent.saveMetadata(data.AgentVersionKey, version.Version)
		agent.saveMetadata(data.AvailabilityZoneKey, agent.availabilityZone)
		agent.saveMetadata(data.ClusterNameKey, agent.cfg.Cluster)
		agent.saveMetadata(data.ContainerInstanceARNKey, agent.containerInstanceARN)
		agent.saveMetadata(data.EC2InstanceIDKey, currentEC2InstanceID)
	}

	// now that we know the container instance ARN, we can build out the doctor
	// and pass it on to ACS and TACS
	doctor, doctorCreateErr := agent.newDoctorWithHealthchecks(agent.cfg.Cluster, agent.containerInstanceARN)
	if doctorCreateErr != nil {
		seelog.Warnf("Error starting doctor, healthchecks won't be running: %v", err)
	} else {
		seelog.Debug("Doctor healthchecks set up properly.")
	}

	// Begin listening to the docker daemon and saving changes
	taskEngine.SetDataClient(agent.dataClient)
	imageManager.SetDataClient(agent.dataClient)
	taskEngine.MustInit(agent.ctx)

	// Start back ground routines, including the telemetry session
	deregisterInstanceEventStream := eventstream.NewEventStream(
		deregisterContainerInstanceEventStreamName, agent.ctx)
	deregisterInstanceEventStream.StartListening()
	taskHandler := eventhandler.NewTaskHandler(agent.ctx, agent.dataClient, state, client)
	attachmentEventHandler := eventhandler.NewAttachmentEventHandler(agent.ctx, agent.dataClient, client)
	agent.startAsyncRoutines(containerChangeEventStream, credentialsManager, imageManager,
		taskEngine, deregisterInstanceEventStream, client, taskHandler, attachmentEventHandler, state, doctor)

	// Start the acs session, which should block doStart
	return agent.startACSSession(credentialsManager, taskEngine,
		deregisterInstanceEventStream, client, state, taskHandler, doctor)
}

// waitUntilInstanceInService Polls IMDS until the target lifecycle state indicates that the instance is going in
// service. This is to avoid instances going to a warm pool being registered as container instances with the cluster
func (agent *ecsAgent) waitUntilInstanceInService(pollWaitDuration time.Duration, pollMaxTimes int, maxRetries int) error {
	seelog.Info("Waiting for instance to go InService")
	var err error
	var targetState string
	// Poll until a target lifecycle state is obtained from IMDS, or an unexpected error occurs
	targetState, err = agent.pollUntilTargetLifecyclePresent(pollWaitDuration, pollMaxTimes, maxRetries)
	if err != nil {
		return err
	}
	// Poll while the instance is in a warmed state until it is going to go into service
	for targetState != inServiceState {
		time.Sleep(pollWaitDuration)
		targetState, err = agent.getTargetLifecycle(maxRetries)
		if err != nil {
			// Do not exit if error is due to throttling or temporary server errors
			// These are likely transient, as at this point IMDS has been successfully queried for state
			switch utils.GetRequestFailureStatusCode(err) {
			case 429, 500, 502, 503, 504:
				seelog.Warnf("Encountered error while waiting for warmed instance to go in service: %v", err)
			default:
				return err
			}
		}
	}
	return err
}

// pollUntilTargetLifecyclePresent polls until obtains a target state or receives an unexpected error
func (agent *ecsAgent) pollUntilTargetLifecyclePresent(pollWaitDuration time.Duration, pollMaxTimes int, maxRetries int) (string, error) {
	var err error
	var targetState string
	for i := 0; i < pollMaxTimes; i++ {
		targetState, err = agent.getTargetLifecycle(maxRetries)
		if targetState != "" ||
			(err != nil && utils.GetRequestFailureStatusCode(err) != 404) {
			break
		}
		time.Sleep(pollWaitDuration)
	}
	return targetState, err
}

// getTargetLifecycle obtains the target lifecycle state for the instance from IMDS. This is populated for instances
// associated with an ASG
func (agent *ecsAgent) getTargetLifecycle(maxRetries int) (string, error) {
	var targetState string
	var err error
	backoff := retry.NewExponentialBackoff(targetLifecycleBackoffMin, targetLifecycleBackoffMax, targetLifecycleBackoffJitter, targetLifecycleBackoffMultiple)
	for i := 0; i < maxRetries; i++ {
		targetState, err = agent.ec2MetadataClient.TargetLifecycleState()
		if err == nil {
			break
		}
		seelog.Debugf("Error when getting intended lifecycle state: %v", err)
		if i < maxRetries {
			time.Sleep(backoff.Duration())
		}
	}
	seelog.Debugf("Target lifecycle state of instance: %v", targetState)
	return targetState, err
}

// newTaskEngine creates a new docker task engine object. It tries to load the
// local state if needed, else initializes a new one
func (agent *ecsAgent) newTaskEngine(containerChangeEventStream *eventstream.EventStream,
	credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	imageManager engine.ImageManager,
	execCmdMgr execcmd.Manager) (engine.TaskEngine, string, error) {

	containerChangeEventStream.StartListening()

	if !agent.cfg.Checkpoint.Enabled() {
		seelog.Info("Checkpointing not enabled; a new container instance will be created each time the agent is run")
		return engine.NewTaskEngine(agent.cfg, agent.dockerClient, credentialsManager,
			containerChangeEventStream, imageManager, state,
			agent.metadataManager, agent.resourceFields, execCmdMgr), "", nil
	}

	savedData, err := agent.loadData(containerChangeEventStream, credentialsManager, state, imageManager, execCmdMgr)
	if err != nil {
		seelog.Criticalf("Error loading previously saved state: %v", err)
		return nil, "", err
	}

	err = agent.checkCompatibility(savedData.taskEngine)
	if err != nil {
		seelog.Criticalf("Error checking compatibility with previously saved state: %v", err)
		return nil, "", err
	}

	currentEC2InstanceID := agent.getEC2InstanceID()
	if savedData.ec2InstanceID != "" && savedData.ec2InstanceID != currentEC2InstanceID {
		seelog.Warnf(instanceIDMismatchErrorFormat,
			savedData.ec2InstanceID, currentEC2InstanceID)

		// Reset agent state as a new container instance
		state.Reset()
		// Reset taskEngine; all the other values are still default
		return engine.NewTaskEngine(agent.cfg, agent.dockerClient, credentialsManager,
			containerChangeEventStream, imageManager, state, agent.metadataManager,
			agent.resourceFields, execCmdMgr), currentEC2InstanceID, nil
	}

	if savedData.cluster != "" {
		if err := agent.setClusterInConfig(savedData.cluster); err != nil {
			return nil, "", err
		}
	}

	// Use the values we loaded if there's no issue
	agent.containerInstanceARN = savedData.containerInstanceARN
	agent.availabilityZone = savedData.availabilityZone
	agent.latestSeqNumberTaskManifest = &savedData.latestTaskManifestSeqNum

	return savedData.taskEngine, currentEC2InstanceID, nil
}

func (agent *ecsAgent) initMetricsEngine() {
	// In case of a panic during set-up, we will recover quietly and resume
	// normal Agent execution.
	defer func() {
		if r := recover(); r != nil {
			seelog.Errorf("MetricsEngine Set-up panicked. Recovering quietly: %s", r)
		}
	}()

	// We init the global MetricsEngine before we publish metrics
	metrics.MustInit(agent.cfg)
	metrics.PublishMetrics()
}

// newDoctorWithHealthchecks creates a new doctor and also configures
// the healthchecks that the doctor should be running
func (agent *ecsAgent) newDoctorWithHealthchecks(cluster, containerInstanceARN string) (*doctor.Doctor, error) {
	// configure the required healthchecks
	runtimeHealthCheck := doctor.NewDockerRuntimeHealthcheck(agent.dockerClient)

	// put the healthechecks in a list
	healthcheckList := []doctor.Healthcheck{
		runtimeHealthCheck,
	}

	// set up the doctor and return it
	return doctor.NewDoctor(healthcheckList, cluster, containerInstanceARN)
}

// setClusterInConfig sets the cluster name in the config object based on
// previous state. It returns an error if there's a mismatch between the
// the current cluster name with what's restored from the cluster state
func (agent *ecsAgent) setClusterInConfig(previousCluster string) error {
	// TODO Handle default cluster in a sane and unified way across the codebase
	configuredCluster := agent.cfg.Cluster
	if configuredCluster == "" {
		seelog.Debug("Setting cluster to default; none configured")
		configuredCluster = config.DefaultClusterName
	}
	if previousCluster != configuredCluster {
		err := clusterMismatchError{
			fmt.Errorf(clusterMismatchErrorFormat, previousCluster, configuredCluster),
		}
		logger.Critical("Error restoring cluster", logger.Fields{
			"previousCluster":   previousCluster,
			"configuredCluster": configuredCluster,
			field.Error:         err,
		})
		return err
	}
	agent.cfg.Cluster = previousCluster
	logger.Info("Cluster was successfully restored", logger.Fields{
		"cluster": agent.cfg.Cluster,
	})

	return nil
}

// getEC2InstanceID gets the EC2 instance ID from the metadata service
func (agent *ecsAgent) getEC2InstanceID() string {
	var instanceID string
	var err error
	backoff := retry.NewExponentialBackoff(instanceIdBackoffMin, instanceIdBackoffMax, instanceIdBackoffJitter, instanceIdBackoffMultiple)
	for i := 0; i < instanceIdMaxRetryCount; i++ {
		instanceID, err = agent.ec2MetadataClient.InstanceID()
		if err == nil || err.Error() == blackholed {
			return instanceID
		}
		if i < instanceIdMaxRetryCount-1 {
			time.Sleep(backoff.Duration())
		}
	}
	if err != nil {
		logger.Warn("Unable to access EC2 Metadata service to determine EC2 ID", logger.Fields{
			field.Error: err,
		})
	}
	return instanceID
}

// getoutpostARN gets the Outpost ARN from the metadata service
func (agent *ecsAgent) getoutpostARN() string {
	outpostARN, err := agent.ec2MetadataClient.OutpostARN()
	if err == nil {
		seelog.Infof(
			"Outpost ARN from EC2 Metadata: %s", outpostARN)
		return outpostARN
	}
	return ""
}

// newStateManager creates a new state manager object for the task engine.
// Rest of the parameters are pointers and it's expected that all of these
// will be backfilled when state manager's Load() method is invoked
func (agent *ecsAgent) newStateManager(
	taskEngine engine.TaskEngine,
	cluster *string,
	containerInstanceArn *string,
	savedInstanceID *string,
	availabilityZone *string, latestSeqNumberTaskManifest *int64) (statemanager.StateManager, error) {
	if !agent.cfg.Checkpoint.Enabled() {
		return statemanager.NewNoopStateManager(), nil
	}

	return agent.stateManagerFactory.NewStateManager(agent.cfg,
		// This is for making testing easier as we can mock this
		agent.saveableOptionFactory.AddSaveable("TaskEngine", taskEngine),
		agent.saveableOptionFactory.AddSaveable("ContainerInstanceArn",
			containerInstanceArn),
		agent.saveableOptionFactory.AddSaveable("Cluster", cluster),
		agent.saveableOptionFactory.AddSaveable("EC2InstanceID", savedInstanceID),
		agent.saveableOptionFactory.AddSaveable("availabilityZone", availabilityZone),
		agent.saveableOptionFactory.AddSaveable("latestSeqNumberTaskManifest", latestSeqNumberTaskManifest),
	)
}

// constructVPCSubnetAttributes returns vpc and subnet IDs of the instance as
// an attribute list
func (agent *ecsAgent) constructVPCSubnetAttributes() []*ecs.Attribute {
	return []*ecs.Attribute{
		{
			Name:  aws.String(vpcIDAttributeName),
			Value: aws.String(agent.vpc),
		},
		{
			Name:  aws.String(subnetIDAttributeName),
			Value: aws.String(agent.subnet),
		},
	}
}

// registerContainerInstance registers the container instance ID for the ECS Agent
func (agent *ecsAgent) registerContainerInstance(
	client api.ECSClient,
	additionalAttributes []*ecs.Attribute) error {
	// Preflight request to make sure they're good
	if preflightCreds, err := agent.credentialProvider.Get(); err != nil || preflightCreds.AccessKeyID == "" {
		seelog.Errorf("Error getting valid credentials: %s", err)
	}

	agentCapabilities, err := agent.capabilities()
	if err != nil {
		return err
	}
	capabilities := append(agentCapabilities, additionalAttributes...)

	// Get the tags of this container instance defined in config file
	tags := utils.MapToTags(agent.cfg.ContainerInstanceTags)
	if agent.cfg.ContainerInstancePropagateTagsFrom == config.ContainerInstancePropagateTagsFromEC2InstanceType {
		ec2Tags, err := agent.getContainerInstanceTagsFromEC2API()
		// If we are unable to call the API, we should not treat it as a transient error,
		// because we've already retried several times, we may throttle the API if we
		// keep retrying.
		if err != nil {
			return err
		}
		seelog.Infof("Retrieved Tags from EC2 DescribeTags API:\n%v", ec2Tags)
		tags = mergeTags(tags, ec2Tags)
	}

	platformDevices := agent.getPlatformDevices()

	outpostARN := agent.getoutpostARN()

	if agent.containerInstanceARN != "" {
		logger.Info("Restored from checkpoint file", logger.Fields{
			"containerInstanceARN": agent.containerInstanceARN,
			"cluster":              agent.cfg.Cluster,
		})
		return agent.reregisterContainerInstance(client, capabilities, tags, uuid.New(), platformDevices, outpostARN)
	}

	logger.Info("Registering Instance with ECS")
	containerInstanceArn, availabilityZone, err := client.RegisterContainerInstance("",
		capabilities, tags, uuid.New(), platformDevices, outpostARN)
	if err != nil {
		logger.Error("Error registering container instance", logger.Fields{
			field.Error: err,
		})
		if retriable, ok := err.(apierrors.Retriable); ok && !retriable.Retry() {
			return err
		}
		if utils.IsAWSErrorCodeEqual(err, ecs.ErrCodeInvalidParameterException) {
			logger.Critical("Instance registration attempt with an invalid parameter", logger.Fields{
				field.Error: err,
			})
			return err
		}
		if _, ok := err.(apierrors.AttributeError); ok {
			attributeErrorMsg := ""
			if len(agent.cfg.InstanceAttributes) > 0 {
				attributeErrorMsg = customAttributeErrorMessage
			}
			logger.Critical("Instance registration attempt with invalid attribute(s)", logger.Fields{
				field.Error: attributeErrorMsg,
			})
			return err
		}
		return transientError{err}
	}
	logger.Info("Instance registration completed successfully", logger.Fields{
		"instanceArn": containerInstanceArn,
		"cluster":     agent.cfg.Cluster,
	})
	agent.containerInstanceARN = containerInstanceArn
	agent.availabilityZone = availabilityZone
	return nil
}

// reregisterContainerInstance registers a container instance that has already been
// registered with ECS. This is for cases where the ECS Agent is being restored
// from a check point.
func (agent *ecsAgent) reregisterContainerInstance(client api.ECSClient, capabilities []*ecs.Attribute,
	tags []*ecs.Tag, registrationToken string, platformDevices []*ecs.PlatformDevice, outpostARN string) error {
	_, availabilityZone, err := client.RegisterContainerInstance(agent.containerInstanceARN, capabilities, tags,
		registrationToken, platformDevices, outpostARN)

	//set az to agent
	agent.availabilityZone = availabilityZone

	if err == nil {
		return nil
	}
	logger.Error("Error re-registering container instance", logger.Fields{
		field.Error: err,
	})
	if apierrors.IsInstanceTypeChangedError(err) {
		seelog.Criticalf(instanceTypeMismatchErrorFormat, err)
		return err
	}
	if _, ok := err.(apierrors.AttributeError); ok {
		attributeErrorMsg := ""
		if len(agent.cfg.InstanceAttributes) > 0 {
			attributeErrorMsg = customAttributeErrorMessage
		}
		logger.Critical("Instance re-registration attempt with invalid attribute(s)", logger.Fields{
			field.Error: attributeErrorMsg,
		})
		return err
	}
	return transientError{err}
}

// startAsyncRoutines starts all of the background methods
func (agent *ecsAgent) startAsyncRoutines(
	containerChangeEventStream *eventstream.EventStream,
	credentialsManager credentials.Manager,
	imageManager engine.ImageManager,
	taskEngine engine.TaskEngine,
	deregisterInstanceEventStream *eventstream.EventStream,
	client api.ECSClient,
	taskHandler *eventhandler.TaskHandler,
	attachmentEventHandler *eventhandler.AttachmentEventHandler,
	state dockerstate.TaskEngineState,
	doctor *doctor.Doctor,
) {

	// Start of the periodic image cleanup process
	if !agent.cfg.ImageCleanupDisabled.Enabled() {
		go imageManager.StartImageCleanupProcess(agent.ctx)
	}

	// Start automatic spot instance draining poller routine
	if agent.cfg.SpotInstanceDrainingEnabled.Enabled() {
		go agent.startSpotInstanceDrainingPoller(agent.ctx, client)
	}

	// Agent introspection api
	go handlers.ServeIntrospectionHTTPEndpoint(agent.ctx, &agent.containerInstanceARN, taskEngine, agent.cfg)

	statsEngine := stats.NewDockerStatsEngine(agent.cfg, agent.dockerClient, containerChangeEventStream)

	// Start serving the endpoint to fetch IAM Role credentials and other task metadata
	if agent.cfg.TaskMetadataAZDisabled {
		// send empty availability zone
		go handlers.ServeTaskHTTPEndpoint(agent.ctx, credentialsManager, state, client, agent.containerInstanceARN, agent.cfg, statsEngine, "", agent.vpc)
	} else {
		go handlers.ServeTaskHTTPEndpoint(agent.ctx, credentialsManager, state, client, agent.containerInstanceARN, agent.cfg, statsEngine, agent.availabilityZone, agent.vpc)
	}

	// Start sending events to the backend
	go eventhandler.HandleEngineEvents(agent.ctx, taskEngine, client, taskHandler, attachmentEventHandler)

	telemetrySessionParams := tcshandler.TelemetrySessionParams{
		Ctx:                           agent.ctx,
		CredentialProvider:            agent.credentialProvider,
		Cfg:                           agent.cfg,
		ContainerInstanceArn:          agent.containerInstanceARN,
		DeregisterInstanceEventStream: deregisterInstanceEventStream,
		ECSClient:                     client,
		TaskEngine:                    taskEngine,
		StatsEngine:                   statsEngine,
		Doctor:                        doctor,
	}

	// Start metrics session in a go routine
	go tcshandler.StartMetricsSession(&telemetrySessionParams)
}

func (agent *ecsAgent) startSpotInstanceDrainingPoller(ctx context.Context, client api.ECSClient) {
	for !agent.spotInstanceDrainingPoller(client) {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Second)
		}
	}
}

// spotInstanceDrainingPoller returns true if spot instance interruption has been
// set AND the container instance state is successfully updated to DRAINING.
func (agent *ecsAgent) spotInstanceDrainingPoller(client api.ECSClient) bool {
	// this endpoint 404s unless a interruption has been set, so expect failure in most cases.
	resp, err := agent.ec2MetadataClient.SpotInstanceAction()
	if err == nil {
		type InstanceAction struct {
			Time   string
			Action string
		}
		ia := InstanceAction{}

		err := json.Unmarshal([]byte(resp), &ia)
		if err != nil {
			seelog.Errorf("Invalid response from /spot/instance-action endpoint: %s Error: %s", resp, err)
			return false
		}

		switch ia.Action {
		case "hibernate", "terminate", "stop":
		default:
			seelog.Errorf("Invalid response from /spot/instance-action endpoint: %s, Error: unrecognized action (%s)", resp, ia.Action)
			return false
		}

		seelog.Infof("Received a spot interruption (%s) scheduled for %s, setting state to DRAINING", ia.Action, ia.Time)
		err = client.UpdateContainerInstancesState(agent.containerInstanceARN, "DRAINING")
		if err != nil {
			seelog.Errorf("Error setting instance [ARN: %s] state to DRAINING: %s", agent.containerInstanceARN, err)
		} else {
			return true
		}
	}
	return false
}

// startACSSession starts a session with ECS's Agent Communication service. This
// is a blocking call and only returns when the handler returns
func (agent *ecsAgent) startACSSession(
	credentialsManager credentials.Manager,
	taskEngine engine.TaskEngine,
	deregisterInstanceEventStream *eventstream.EventStream,
	client api.ECSClient,
	state dockerstate.TaskEngineState,
	taskHandler *eventhandler.TaskHandler,
	doctor *doctor.Doctor) int {

	acsSession := acshandler.NewSession(
		agent.ctx,
		agent.cfg,
		deregisterInstanceEventStream,
		agent.containerInstanceARN,
		agent.credentialProvider,
		agent.dockerClient,
		client,
		state,
		agent.dataClient,
		taskEngine,
		credentialsManager,
		taskHandler,
		agent.latestSeqNumberTaskManifest,
		doctor,
	)
	seelog.Info("Beginning Polling for updates")
	err := acsSession.Start()
	if err != nil {
		seelog.Criticalf("Unretriable error starting communicating with ACS: %v", err)
		return exitcodes.ExitTerminal
	}
	return exitcodes.ExitSuccess
}

// validateRequiredVersion validates docker version.
// Minimum docker version supported is 1.9.0, maps to api version 1.21
// see https://docs.docker.com/develop/sdk/#api-version-matrix
func (agent *ecsAgent) verifyRequiredDockerVersion() (int, bool) {
	supportedVersions := agent.dockerClient.SupportedVersions()
	if len(supportedVersions) == 0 {
		seelog.Critical("Could not get supported docker versions.")
		return exitcodes.ExitError, false
	}

	// if api version 1.21 is supported, it means docker version is at least 1.9.0
	for _, version := range supportedVersions {
		if version == dockerclient.Version_1_21 {
			return -1, true
		}
	}

	// api 1.21 is not supported, docker version is older than 1.9.0
	seelog.Criticalf("Required minimum docker API verion %s is not supported",
		dockerclient.Version_1_21)
	return exitcodes.ExitTerminal, false
}

// getContainerInstanceTagsFromEC2API will retrieve the tags of this instance remotely.
func (agent *ecsAgent) getContainerInstanceTagsFromEC2API() ([]*ecs.Tag, error) {
	// Get instance ID from ec2 metadata client.
	instanceID, err := agent.ec2MetadataClient.InstanceID()
	if err != nil {
		return nil, err
	}

	return agent.ec2Client.DescribeECSTagsForInstance(instanceID)
}

// mergeTags will merge the local tags and ec2 tags, for the overlap part, ec2 tags
// will be overridden by local tags.
func mergeTags(localTags []*ecs.Tag, ec2Tags []*ecs.Tag) []*ecs.Tag {
	tagsMap := make(map[string]string)

	for _, ec2Tag := range ec2Tags {
		tagsMap[aws.StringValue(ec2Tag.Key)] = aws.StringValue(ec2Tag.Value)
	}

	for _, localTag := range localTags {
		tagsMap[aws.StringValue(localTag.Key)] = aws.StringValue(localTag.Value)
	}

	return utils.MapToTags(tagsMap)
}

// getHostPrivateIPv4AddressFromEC2Metadata will retrieve the PrivateIPAddress (IPv4) of this
// instance throught the EC2 API
func (agent *ecsAgent) getHostPrivateIPv4AddressFromEC2Metadata() string {
	// Get instance private IP from ec2 metadata client.
	hostPrivateIPv4Address, err := agent.ec2MetadataClient.PrivateIPv4Address()
	if err != nil {
		seelog.Errorf("Unable to retrieve Host Instance PrivateIPv4 Address: %v", err)
		return ""
	}
	return hostPrivateIPv4Address
}

// getHostPublicIPv4AddressFromEC2Metadata will retrieve the PublicIPAddress (IPv4) of this
// instance through the EC2 API
func (agent *ecsAgent) getHostPublicIPv4AddressFromEC2Metadata() string {
	// Get instance public IP from ec2 metadata client.
	hostPublicIPv4Address, err := agent.ec2MetadataClient.PublicIPv4Address()
	if err != nil {
		seelog.Errorf("Unable to retrieve Host Instance PublicIPv4 Address: %v", err)
		return ""
	}
	return hostPublicIPv4Address
}

func (agent *ecsAgent) saveMetadata(key, val string) {
	err := agent.dataClient.SaveMetadata(key, val)
	if err != nil {
		seelog.Errorf("Failed to save agent metadata to disk (key: [%s], value: [%s]): %v", key, val, err)
	}
}

// setVPCSubnet sets the vpc and subnet ids for the agent by querying the
// instance metadata service
func (agent *ecsAgent) setVPCSubnet() (error, bool) {
	mac, err := agent.ec2MetadataClient.PrimaryENIMAC()
	if err != nil {
		return fmt.Errorf("unable to get mac address of instance's primary ENI from instance metadata: %v", err), false
	}

	vpcID, err := agent.ec2MetadataClient.VPCID(mac)
	if err != nil {
		if isInstanceLaunchedInVPC(err) {
			return fmt.Errorf("unable to get vpc id from instance metadata: %v", err), true
		}
		return instanceNotLaunchedInVPCError, false
	}

	subnetID, err := agent.ec2MetadataClient.SubnetID(mac)
	if err != nil {
		return fmt.Errorf("unable to get subnet id from instance metadata: %v", err), false
	}

	agent.vpc = vpcID
	agent.subnet = subnetID
	agent.mac = mac

	return nil, false
}

// isInstanceLaunchedInVPC returns false when the awserr returned is an EC2MetadataError
// when querying the vpc id from instance metadata
func isInstanceLaunchedInVPC(err error) bool {
	if aerr, ok := err.(awserr.Error); ok &&
		aerr.Code() == "EC2MetadataError" {
		return false
	}
	return true
}

// contains is a comparision function which checks if the target string is present in the array
func contains(capabilities []string, capability string) bool {
	for _, cap := range capabilities {
		if cap == capability {
			return true
		}
	}

	return false
}
