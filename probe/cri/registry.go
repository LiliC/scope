package cri

import (
	"fmt"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/armon/go-radix"

	"github.com/weaveworks/scope/probe/controls"
	"github.com/weaveworks/scope/report"
	criClient "github.com/weaveworks/scope/runtime"
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
	WalkImages(f func(criClient.Image))
	//WalkNetworks(f func(docker_client.Network))
	WatchContainerUpdates(ContainerUpdateWatcher)
	GetContainer(string) (Container, bool)
	GetContainerByPrefix(string) (Container, bool)
	GetContainerImage(string) (criClient.Image, bool)
}

// ContainerUpdateWatcher is the type of functions that get called when containers are updated.
type ContainerUpdateWatcher func(report.Node)

type registry struct {
	sync.RWMutex
	quit                   chan chan struct{}
	interval               time.Duration
	collectStats           bool
	client                 criClient.RuntimeServiceClient
	pipes                  controls.PipeClient
	hostID                 string
	handlerRegistry        *controls.HandlerRegistry
	noCommandLineArguments bool
	noEnvironmentVariables bool

	watchers []ContainerUpdateWatcher

	containers *radix.Tree
	// TODO: implement this in container.go
	containersByPID map[int]Container
	images          map[string]criClient.Image
	pipeIDToexecID  map[string]string
	networks        map[string]string
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
		images:          map[string]*criClient.Image{},
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

	// TODO: implement this in controls.go
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

func (r *registry) loop() {
	for {
		// NB listenForEvents blocks.
		// Returning false means we should exit.
		if !r.listenForEvents() {
			return
		}

		// Sleep here so we don't hammer the
		// logs if docker is down
		time.Sleep(r.interval)
	}
}

func (r *registry) listenForEvents() bool {
	// First we empty the store lists.
	// This ensure any containers that went away in between calls to
	// listenForEvents don't hang around.
	r.reset()

	// Next, start listening for events.  We do this before fetching
	// the list of containers so we don't miss containers created
	// after listing but before listening for events.

	/* TODO: implemnet listening?
	// TODO: left here ----
	events := make(chan *docker_client.APIEvents)
	if err := r.client.AddEventListener(events); err != nil {
		log.Errorf("cri registry: %s", err)
		return true
	}
	defer func() {
		if err := r.client.RemoveEventListener(events); err != nil {
			log.Errorf("cri registry: %s", err)
		}
	}()


	if err := r.updateContainers(); err != nil {
		log.Errorf("cri registry: %s", err)
		return true
	}

	if err := r.updateImages(); err != nil {
		log.Errorf("cri registry: %s", err)
		return true
	}

	if err := r.updateNetworks(); err != nil {
		log.Errorf("cri registry: %s", err)
		return true
	}
	*/

	otherUpdates := time.Tick(r.interval)
	for {
		select {
		/*
			case event, ok := <-events:
				if !ok {
					log.Errorf("cri registry: event listener unexpectedly disconnected")
					return true
				}
				r.handleEvent(event)
		*/
		case <-otherUpdates:
			if err := r.updateContainers(); err != nil {
				log.Errorf("cri registry: %s", err)
				return true
			}

			if err := r.updateImages(); err != nil {
				log.Errorf("cri registry: %s", err)
				return true
			}
			// TODO: CNI?
			if err := r.updateNetworks(); err != nil {
				log.Errorf("cri registry: %s", err)
				return true
			}

		case ch := <-r.quit:
			r.Lock()
			defer r.Unlock()

			if r.collectStats {
				r.containers.Walk(func(_ string, c interface{}) bool {
					c.(Container).StopGatheringStats()
					return false
				})
			}
			close(ch)
			return false
		}
	}
}

func (r *registry) reset() {
	r.Lock()
	defer r.Unlock()

	if r.collectStats {
		r.containers.Walk(func(_ string, c interface{}) bool {
			c.(Container).StopGatheringStats()
			return false
		})
	}

	r.containers = radix.New()
	r.containersByPID = map[int]Container{}
	r.images = map[string]criClient.Images{}
	r.networks = r.networks[:0]
}

func (r *registry) updateContainers() error {
	// apiContainers, err := r.client.ListContainersRequest(docker_client.ListContainersOptions{All: true})
	containers, err := r.client.ListContainers(criClient.ListContainersRequest{})
	if err != nil {
		return err
	}

	for _, container := range containers {
		r.updateContainerState(container.ID, nil)
	}

	return nil
}

func (r *registry) updateImages() error {
	images, err := r.client.ListImages(criClient.ListImagesRequest{})
	if err != nil {
		return err
	}

	r.Lock()
	defer r.Unlock()

	for _, image := range images {
		r.images[trimImageID(image.ID)] = image
	}

	return nil
}

// TODO: CRI does not do network
// TODO: figure out a way to do this - via CNI lib?
func (r *registry) updateNetworks() error {
	/*
		networks, err := r.client.ListNetworks()
		if err != nil {
			return err
		}
	*/

	r.Lock()
	r.networks = nil
	r.Unlock()

	return nil
}

/*
func (r *registry) handleEvent(event *docker_client.APIEvents) {
	// TODO: Send shortcut reports on networks being created/destroyed?
	switch event.Status {
	case CreateEvent, RenameEvent, StartEvent, DieEvent, DestroyEvent, PauseEvent, UnpauseEvent, NetworkConnectEvent, NetworkDisconnectEvent:
		r.updateContainerState(event.ID, stateAfterEvent(event.Status))
	}
}
*/
func stateAfterEvent(event string) *string {
	switch event {
	case DestroyEvent:
		return &StateDeleted
	default:
		return nil
	}
}

func (r *registry) updateContainerState(containerID string, intendedState *string) {
	r.Lock()
	defer r.Unlock()

	container, err := r.client.ContainerStatus(criClient.ContainerStatusRequest{id: containerId})
	if err != nil {
		// Container doesn't exist anymore, so lets stop and remove it

		// We have to first stop the container and remove it afterwards
		r.client.StopContainer(criClient.StopContainerRequest{id: conainerID})

		r.client.RemoveContainer(criClient.RemoveContainerRequest{id: containerID})

		delete(r.containersByPID, container.PID())
		if r.collectStats {
			container.StopGatheringStats()
		}

		if intendedState != nil {
			node := report.MakeNodeWith(report.MakeContainerNodeID(containerID), map[string]string{
				ContainerID:    containerID,
				ContainerState: *intendedState,
			})
			// Trigger anyone watching for updates
			for _, f := range r.watchers {
				f(node)
			}
		}
		return
	}

	// Container exists, ensure we have it
	o, ok := r.containers.Get(containerID)
	var c Container
	if !ok {
		c = NewContainerStub(dockerContainer, r.hostID, r.noCommandLineArguments, r.noEnvironmentVariables)
		r.containers.Insert(containerID, c)
	} else {
		c = o.(Container)
		// potentially remove existing pid mapping.
		delete(r.containersByPID, c.PID())
		c.UpdateState(dockerContainer)
	}

	// Update PID index
	if c.PID() > 1 {
		r.containersByPID[c.PID()] = c
	}

	// Trigger anyone watching for updates
	node := c.GetNode()
	for _, f := range r.watchers {
		f(node)
	}

	// And finally, ensure we gather stats for it
	if r.collectStats {
		if dockerContainer.State.Running {
			if err := c.StartGatheringStats(r.client); err != nil {
				log.Errorf("Error gathering stats for container %s: %s", containerID, err)
				return
			}
		} else {
			c.StopGatheringStats()
		}
	}
}

// LockedPIDLookup runs f under a read lock, and gives f a function for
// use doing pid->container lookups.
func (r *registry) LockedPIDLookup(f func(func(int) Container)) {
	r.RLock()
	defer r.RUnlock()

	lookup := func(pid int) Container {
		return r.containersByPID[pid]
	}

	f(lookup)
}

// WalkContainers runs f on every running containers the registry knows of.
func (r *registry) WalkContainers(f func(Container)) {
	r.RLock()
	defer r.RUnlock()

	r.containers.Walk(func(_ string, c interface{}) bool {
		f(c.(Container))
		return false
	})
}

func (r *registry) GetContainer(id string) (Container, bool) {
	r.RLock()
	defer r.RUnlock()
	c, ok := r.containers.Get(id)
	if ok {
		return c.(Container), true
	}
	return nil, false
}

func (r *registry) GetContainerByPrefix(prefix string) (Container, bool) {
	r.RLock()
	defer r.RUnlock()
	out := []interface{}{}
	r.containers.WalkPrefix(prefix, func(_ string, v interface{}) bool {
		out = append(out, v)
		return false
	})
	if len(out) == 1 {
		return out[0].(Container), true
	}
	return nil, false
}

func (r *registry) GetContainerImage(id string) (criClient.Image, bool) {
	r.RLock()
	defer r.RUnlock()
	image, ok := r.images[id]
	return image, ok
}

// WalkImages runs f on every image of running containers the registry
// knows of.  f may be run on the same image more than once.
func (r *registry) WalkImages(f func([]criClient.Image)) {
	r.RLock()
	defer r.RUnlock()

	// Loop over containers so we only emit images for running containers.
	r.containers.Walk(func(_ string, c interface{}) bool {
		image, ok := r.images[c.(Container).Image()]
		if ok {
			f(image)
		}
		return false
	})
}

/*
// WalkNetworks runs f on every network the registry knows of.
func (r *registry) WalkNetworks(f func(docker_client.Network)) {
	r.RLock()
	defer r.RUnlock()

	for _, network := range r.networks {
		f(network)
	}
}
*/

// ImageNameWithoutVersion splits the image name apart, returning the name
// without the version, if possible
func ImageNameWithoutVersion(name string) string {
	parts := strings.SplitN(name, "/", 3)
	if len(parts) == 3 {
		name = fmt.Sprintf("%s/%s", parts[1], parts[2])
	}
	parts = strings.SplitN(name, ":", 2)
	return parts[0]
}

// TODO: listen for events
// Implement!
func (r *registry) addEventListener(events string) error {
	fmt.Println("listening events...")
}
