package cri

import (
	"fmt"
)

// Vars exported for testing.
var (
	NewCRIClientStub = newCRIClient
	// TODO: create in container.go
	NewContainerStub = NewContainer
)

// Registry keeps track of running docker containers and their images
type Registry interface {
	Stop()
	LockedPIDLookup(f func(func(int) Container))
	WalkContainers(f func(Container))
	WalkImages(f func(runtime.Image))
	WalkNetworks(f func(docker_client.Network))
	WatchContainerUpdates(ContainerUpdateWatcher)
	GetContainer(string) (Container, bool)
	GetContainerByPrefix(string) (Container, bool)
	GetContainerImage(string) (runtime.Image, bool)
}

// ContainerUpdateWatcher is the type of functions that get called when containers are updated.
type ContainerUpdateWatcher func(report.Node)

type registry struct {
	sync.RWMutex
	quit                   chan chan struct{}
	interval               time.Duration
	collectStats           bool
	client                 criClient.RunttimeServiceClient
	pipes                  controls.PipeClient
	hostID                 string
	handlerRegistry        *controls.HandlerRegistry
	noCommandLineArguments bool
	noEnvironmentVariables bool

	watchers   []ContainerUpdateWatcher
	containers *radix.Tree
	// TODO: implement this in container.go
	containersByPID map[int]Container
	images          map[string]criClient.Image
	networks        []docker_client.Network
	pipeIDToexecID  map[string]string
}

// TODO: criClient.RuntimeServiceClient
// Client interface for mocking.

func newCRIClient(endpoint string) (criClient.RuntimeServiceClient, error) {
	if endpoint == "" {
		// We default to docker endpoint
		endpoint = "unix///var/run/dockershim.sock"
	}

	// Dial grpc endpoint
	addr, dailer, err := GetAddressAndDialer(endpoint)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDialer(dailer))
	if err != nil {
		return nil, err
	}
	// TODO: should we handle the connection here?
	defer conn.Close()

	return criClient.NewRuntimeServiceClient(conn), nil
}

// RegistryOptions are used to initialize the Registry
type RegistryOptions struct {
	Interval               time.Duration
	Pipes                  controls.PipeClient
	CollectStats           bool
	HostID                 string
	HandlerRegistry        *controls.HandlerRegistry
	CRIEndpoint            string
	NoCommandLineArguments bool
	NoEnvironmentVariables bool
}

// NewRegistry returns a usable Registry. Don't forget to Stop it.
func NewRegistry(options RegistryOptions) (Registry, error) {
	client, err := NewCRIClientStub(options.CRIEndpoint)
	if err != nil {
		return nil, err
	}

	r := &registry{
		containers:      radix.New(),
		containersByPID: map[int]Container{},
		images:          map[string]docker_client.APIImages{},
		pipeIDToexecID:  map[string]string{},

		client:          client,
		pipes:           options.Pipes,
		interval:        options.Interval,
		collectStats:    options.CollectStats,
		hostID:          options.HostID,
		handlerRegistry: options.HandlerRegistry,
		quit:            make(chan chan struct{}),
		noCommandLineArguments: options.NoCommandLineArguments,
		noEnvironmentVariables: options.NoEnvironmentVariables,
	}

	r.registerControls()
	go r.loop()
	return r, nil
}

// Stop stops the Docker registry's event subscriber.
func (r *registry) Stop() {
	r.deregisterControls()
	ch := make(chan struct{})
	r.quit <- ch
	<-ch
}
