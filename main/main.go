package main

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"fmt"
	"github.com/lestrrat-go/backoff"
	"github.com/skysoft-atm/gorillaz"
	"github.com/skysoft-atm/supercaster/network"
	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"strings"
	"time"
)

var backoffPolicy = backoff.NewExponential(
	backoff.WithInterval(500*time.Millisecond),
	backoff.WithMaxInterval(5*time.Second),
	backoff.WithJitterFactor(0.05),
	backoff.WithMaxRetries(0),
)

func main() {

	project, zone, err := getProjectAndZone()
	if err != nil {
		gorillaz.Log.Warn("Could not find project and zone", zap.Error(err))
	}

	ports := getPortsToListen(project, zone)
	gorillaz.Sugar.Infof("Going to listen to ports %s and send them as unicast to all VMs on this project", strings.Join(ports, ","))

	tick := time.NewTicker(2 * time.Minute)

	currentInstances := make(map[string]instanceHandle)

	for range tick.C {
		freshInstances, err := getInstances(project, zone)
		if err != nil {
			gorillaz.Log.Warn("Error while getting instances", zap.Error(err))
		}
		newInstances := getNewInstances(currentInstances, freshInstances)
		if len(newInstances) > 0 {
			currentInstances = startNewInstances(currentInstances, newInstances, ports)
		}

	}

}

func getPortsToListen(project, zone string) []string {
	bo, cancel := backoffPolicy.Start(context.Background())
	defer cancel()
	for backoff.Continue(bo) {
		instances, err := getInstances(project, zone)
		if err != nil {
			gorillaz.Log.Warn("Could not get instances", zap.Error(err))
			continue
		}
		addresses, err := network.GetNetworkInterfaceAddresses()
		if err != nil {
			gorillaz.Log.Warn("Could not get addresses", zap.Error(err))
			continue
		}
		vm, err := getMatchingInstance(instances, addresses)
		if err != nil {
			gorillaz.Log.Warn("Could not get matching instance", zap.Error(err))
			continue
		}
		gorillaz.Sugar.Infof("We are running on instance %s", vm.name)
		ports, ok := vm.labels["broadcast-ports"]
		if ok {
			return strings.Split(ports, "_")
		} else {
			otherLabels := make([]string, len(vm.labels))
			i := 0
			for l, _ := range vm.labels {
				otherLabels[i] = l
				i++
			}
			gorillaz.Log.Info("broadcast-ports label not found", zap.Error(err), zap.Strings("otherLabels", otherLabels))
		}
	}
	return nil
}

func getMatchingInstance(vms instances, addresses map[string][]string) (*vmInstance, error) {
	for _, vm := range vms {
		for _, vmIp := range vm.addresses {
			for _, addresses := range addresses {
				for _, addr := range addresses {
					if addr == vmIp {
						return &vm, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("instance not found")
}

func startNewInstances(currentInstances map[string]instanceHandle, newInstances instances, ports []string) map[string]instanceHandle {

	//TODO
	//for k, v := range newInstances {
	//
	//}
	return nil

}

type instanceHandle struct {
	ips    []string
	cancel context.CancelFunc
}

func getNewInstances(currentInstances map[string]instanceHandle, freshInstances instances) instances {
	res := make(map[string]vmInstance)
	for k, v := range freshInstances {
		_, ok := currentInstances[k]
		if !ok {
			res[k] = v
		}
	}
	return res
}

func getDeletedInstances(currentInstances map[string]instanceHandle, freshInstances instances) []string {
	res := make([]string, 0)
	for k, _ := range currentInstances {
		_, ok := freshInstances[k]
		if !ok {
			res = append(res, k)
		}
	}
	return res
}

type instances map[string]vmInstance

type vmInstance struct {
	name      string
	addresses []string
	labels    map[string]string
}

func getInstances(project, zone string) (instances, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	cs, err := compute.NewService(ctx)
	if err != nil {
		return nil, err
	}

	il, err := cs.Instances.List(project, zone).Do()
	if err != nil {
		return nil, err
	}

	res := make(map[string]vmInstance)

	for _, it := range il.Items {
		ips := make([]string, len(it.NetworkInterfaces))
		for i, ni := range it.NetworkInterfaces {
			ips[i] = ni.NetworkIP
		}
		vm := vmInstance{
			name:      it.Name,
			addresses: ips,
			labels:    it.Labels,
		}
		res[it.Name] = vm
	}
	return res, nil
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
