package cri

import (
	"context"
	"fmt"

	"github.com/weaveworks/scope/probe/docker"
	"github.com/weaveworks/scope/report"
	criClient "github.com/weaveworks/scope/runtime"
)

// Reporter generate Reports containing Container and ContainerImage topologies
type Reporter struct {
	cri criClient.RuntimeServiceClient
}

// NewReporter makes a new Reporter
func NewReporter(cri criClient.RuntimeServiceClient) *Reporter {
	reporter := &Reporter{
		cri: cri,
	}
	fmt.Println("report?")
	return reporter
}

// Name of this reporter, for metrics gathering
func (Reporter) Name() string { return "CRI" }

// Report generates a Report containing Container and ContainerImage topologies
func (r *Reporter) Report() (report.Report, error) {
	result := report.MakeReport()
	containerTopol, err := r.containerTopology()
	if err != nil {
		return report.MakeReport(), err
	}

	result.Container = result.Container.Merge(containerTopol)
	return result, nil
}

func (r *Reporter) containerTopology() (report.Topology, error) {
	fmt.Println("container topology...")
	result := report.MakeTopology().
		WithMetadataTemplates(docker.ContainerImageMetadataTemplates).
		WithTableTemplates(docker.ContainerImageTableTemplates)

	resp, err := r.cri.ListContainers(context.TODO(), &criClient.ListContainersRequest{})
	if err != nil {
		return result, err
	}

	for _, c := range resp.Containers {
		fmt.Println("container:")
		fmt.Println(c)
		result.AddNode(getBaseNode(c))
	}
	fmt.Println("node result:")
	fmt.Println(result)
	return result, nil
}

func getBaseNode(c *criClient.Container) report.Node {
	result := report.MakeNodeWith(report.MakeContainerNodeID(c.Id), map[string]string{
		docker.ContainerName:         c.Metadata.Name,
		docker.ContainerID:           c.Id,
		docker.ContainerCreated:      fmt.Sprintf("%v", c.CreatedAt),
		docker.ContainerState:        fmt.Sprintf("%v", c.State),
		docker.ContainerRestartCount: fmt.Sprintf("%v", c.Metadata.Attempt),
		docker.ImageID:               c.ImageRef,
		docker.ImageName:             c.Image.Image,
	}).WithParents(report.MakeSets().
		Add(report.ContainerImage, report.MakeStringSet(report.MakeContainerImageNodeID(c.ImageRef))),
	)
	result = result.AddPrefixPropertyList(docker.LabelPrefix, c.Labels)
	return result
}
