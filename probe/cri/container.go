package cri

import (
	"fmt"
	"sync"

	"github.com/weaveworks/scope/report"
	runtime "github.com/weaveworks/scope/runtime"
)

// Container represents a CRI container
type Container interface {
	UpdateState(*runtime.Container)

	ID() string
	Image() string
	PID() int
	Hostname() string
	GetNode() report.Node
	State() string
	StateString() string
	HasTTY() bool
	Container() *runtime.Container
	StartGatheringStats(StatsGatherer) error
	StopGatheringStats()
}

type container struct {
	sync.RWMutex
	container              *runtime.Container
	stopStats              chan<- bool
	latestStats            runtime.ContainerStats
	pendingStats           [60]runtime.ContainerStats
	numPending             int
	hostID                 string
	baseNode               report.Node
	noCommandLineArguments bool
	noEnvironmentVariables bool
}

// StatsGatherer gathers container stats
type StatsGatherer interface {
	Stats(runtime.ContainerStatsRequest) error
}

// NewContainer creates a new Container
func NewContainer(c *runtime.Container, hostID string, noCommandLineArguments bool, noEnvironmentVariables bool) Container {
	result := &container{
		container:              c,
		hostID:                 hostID,
		noCommandLineArguments: noCommandLineArguments,
		noEnvironmentVariables: noEnvironmentVariables,
	}
	result.baseNode = result.getBaseNode()
	return result
}

func (c *container) ID() string {
	// runtime.CreateContainerRequest.PodSandboxId
	return c.container.Id
}

func (c *container) Image() string {

	return trimImageID(c.container.Image)
}

// TODO: no PID
func (c *container) PID() int {
	return 1
}

func (c *container) Hostname() string {
	if c.container.PodSandboxConfig.DnsConfig == "" {
		return c.container.Config.Hostname
	}

	return fmt.Sprintf("%s.%s", c.container.PodSandboxConfig.Hostname,
		c.container.PodSandboxConfig.DnsConfig)
}

func (c *container) HasTTY() bool {
	return c.container.AttachRequest.Tty
}

func (c *container) State() string {
	return c.container.State.String()
}

func (c *container) StateString() string {
	return c.container.State.StateString()
}

func (c *container) Container() *runtime.Container {
	return c.container
}

func (c *container) getBaseNode() report.Node {
	result := report.MakeNodeWith(report.MakeContainerNodeID(c.ID()), map[string]string{
		ContainerID:       c.ID(),
		ContainerCreated:  c.container.Created.Format(time.RFC3339Nano),
		ContainerCommand:  c.getSanitizedCommand(),
		ImageID:           c.Image(),
		ContainerHostname: c.Hostname(),
	}).WithParents(report.MakeSets().
		Add(report.ContainerImage, report.MakeStringSet(report.MakeContainerImageNodeID(c.Image()))),
	)
	result = result.AddPrefixPropertyList(LabelPrefix, c.container.Config.Labels)
	if !c.noEnvironmentVariables {
		result = result.AddPrefixPropertyList(EnvPrefix, c.env())
	}
	return result
}

func (c *container) GetNode() report.Node {
	c.RLock()
	defer c.RUnlock()
	latest := map[string]string{
		ContainerName:       strings.TrimPrefix(c.container.Name, "/"),
		ContainerState:      c.StateString(),
		ContainerStateHuman: c.State(),
	}
	controls := c.controlsMap()

	if !c.container.State.Paused && c.container.State.Running {
		uptimeSeconds := int(mtime.Now().Sub(c.container.State.StartedAt) / time.Second)
		networkMode := ""
		if c.container.HostConfig != nil {
			networkMode = c.container.HostConfig.NetworkMode
		}
		latest[ContainerUptime] = strconv.Itoa(uptimeSeconds)
		latest[ContainerRestartCount] = strconv.Itoa(c.container.RestartCount)
		latest[ContainerNetworkMode] = networkMode
	}

	result := c.baseNode.WithLatests(latest)
	result = result.WithLatestControls(controls)
	result = result.WithMetrics(c.metrics())
	return result
}

func (c *container) StartGatheringStats(client StatsGatherer) error {
	c.Lock()
	defer c.Unlock()

	if c.stopStats != nil {
		return nil
	}
	done := make(chan bool)
	c.stopStats = done

	stats := make(chan *docker.Stats)
	opts := docker.StatsOptions{
		ID:     c.container.ID,
		Stats:  stats,
		Stream: true,
		Done:   done,
	}

	log.Debugf("docker container: collecting stats for %s", c.container.ID)

	go func() {
		if err := client.Stats(opts); err != nil && err != io.EOF && err != io.ErrClosedPipe {
			log.Errorf("docker container: error collecting stats for %s: %v", c.container.ID, err)
		}
	}()

	go func() {
		for s := range stats {
			c.Lock()
			if c.numPending >= len(c.pendingStats) {
				log.Warnf("docker container: dropping stats for %s", c.container.ID)
			} else {
				c.latestStats = *s
				c.pendingStats[c.numPending] = *s
				c.numPending++
			}
			c.Unlock()
		}
		log.Debugf("docker container: stopped collecting stats for %s", c.container.ID)
		c.Lock()
		if c.stopStats == done {
			c.stopStats = nil
		}
		c.Unlock()
	}()

	return nil
}
