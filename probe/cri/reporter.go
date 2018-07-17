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
	imageTopology, err := r.containerImageTopology()
	if err != nil {
		return report.MakeReport(), err
	}

	result.ContainerImage = result.ContainerImage.Merge(imageTopology)
	return result, nil
}

func (r *Reporter) containerImageTopology() (report.Topology, error) {
	fmt.Println("containerimagetopology...")
	result := report.MakeTopology().
		WithMetadataTemplates(docker.ContainerImageMetadataTemplates).
		WithTableTemplates(docker.ContainerImageTableTemplates)

	resp, err := r.cri.ListContainers(context.TODO(), &criClient.ListContainersRequest{})
	if err != nil {
		return result, err
	}

	for _, c := range resp.Containers {
		/*	fmt.Println(c)
			latests := map[string]string{
				docker.ImageID:          c.ImageRef,
				docker.ImageSize:        "10MB",
				docker.ImageVirtualSize: "10MB",
				docker.ImageName:        c.Image.Image,
			}
			nodeID := report.MakeContainerImageNodeID(latests[docker.ImageID])
			node := report.MakeNodeWith(nodeID, latests)
		*/
		result.AddNode(getBaseNode(c))
	}
	return result, nil
}

func getBaseNode(c *criClient.Container) report.Node {
	result := report.MakeNodeWith(report.MakeContainerNodeID(c.Id), map[string]string{
		//docker.ContainerName:     c.Metadata.Name,
		docker.ContainerID:       c.Id,
		docker.ContainerCreated:  fmt.Sprintf("%v", c.CreatedAt),
		docker.ContainerCommand:  "/bin/bash",
		docker.ImageID:           c.ImageRef,
		docker.ContainerHostname: "host",
	}).WithParents(report.MakeSets().
		Add(report.ContainerImage, report.MakeStringSet(report.MakeContainerImageNodeID(c.ImageRef))),
	)
	result = result.AddPrefixPropertyList(docker.LabelPrefix, c.Labels)
	return result
}
