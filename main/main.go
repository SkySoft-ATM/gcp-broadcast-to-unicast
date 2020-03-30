package main

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"time"
)

func main() {
	project, zone, err := getProjectAndZone()

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

func getProjectAndZone() (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	cli, err := google.DefaultClient(ctx)
	if err != nil {
		return "", "", err
	}
	c := metadata.NewClient(cli)
	p, err := c.ProjectID()
	if err != nil {
		return "", "", err
	}

	zone, err := c.Zone()
	return p, zone, nil
}
