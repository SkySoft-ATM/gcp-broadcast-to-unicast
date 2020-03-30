package main

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"time"
)

// This example demonstrates how to use your own transport when using this package.
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	cli, err := google.DefaultClient(ctx)
	if err != nil {
		panic(err)
	}
	c := metadata.NewClient(cli)
	p, err := c.ProjectID()
	if err != nil {
		panic(err)
	}
	fmt.Println("Working on project", p)

	project := p
	zone, err := c.Zone()
	fmt.Println("in zone", zone)
	cancel()

	cs, err := compute.NewService(context.Background())
	if err != nil {
		panic(err)
	}

	il, err := cs.Instances.List(project, zone).Do()
	if err != nil {
		panic(err)
	}
	for _, i := range il.Items {

		fmt.Println("instance", i.Name, i.Labels)
		for _, ni := range i.NetworkInterfaces {
			fmt.Println(" >>>", ni.NetworkIP)
		}
	}
}
