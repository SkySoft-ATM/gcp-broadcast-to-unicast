package main

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"fmt"
	"github.com/lestrrat-go/backoff"
	"github.com/skysoft-atm/gorillaz"
	"github.com/skysoft-atm/gorillaz/mux"
	"github.com/skysoft-atm/supercaster/broadcast"
	"github.com/skysoft-atm/supercaster/network"
	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"strconv"
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

	vm := getVmInstance(project, zone)
	gorillaz.Sugar.Infof("We are running on instance %s", vm.name)
	ports, maxDatagramSize := getPortsToListen(vm)
	gorillaz.Sugar.Infof("Going to listen to ports %s and send them as unicast to all VMs on this project", strings.Join(ports, ","))

	broadcasters := make([]udpBroadcaster, len(ports))
	for i, port := range ports {
		broadcasters[i] = createStreamForPort(port, maxDatagramSize)
	}

	gorillaz.Log.Info("UDP listening on ports " + strings.Join(ports, ","))

	//tick := time.NewTicker(2 * time.Minute)
	tick := time.NewTicker(20 * time.Second)

	currentInstances := make(map[string]instanceHandle)

	refreshInstances(project, zone, vm.name, currentInstances, broadcasters)

	for range tick.C {
		refreshInstances(project, zone, vm.name, currentInstances, broadcasters)
	}

}

func refreshInstances(project string, zone string, instanceName string, currentInstances map[string]instanceHandle, broadcasters []udpBroadcaster) {
	freshInstances, err := getInstances(project, zone)
	delete(freshInstances, instanceName) // we exclude our own instance
	if err != nil {
		gorillaz.Log.Warn("Error while getting instances", zap.Error(err))
	}
	newInstances := getNewInstances(currentInstances, freshInstances)
	if len(newInstances) > 0 {
		addAndStartNewInstances(currentInstances, newInstances, broadcasters)
	}
	deleted := getDeletedInstances(currentInstances, freshInstances)
	for _, d := range deleted {
		ih, ok := currentInstances[d]
		if ok {
			gorillaz.Sugar.Infof("Cancelling the publication for VM instance %s", d)
			ih.cancel()
			delete(currentInstances, d)
		}
	}
}

type udpBroadcaster struct {
	*mux.Broadcaster
	port string
}

func createStreamForPort(port string, maxDatagramSize int) udpBroadcaster {
	b := mux.NewNonBlockingBroadcaster(100)

	go func() {
		bo, cancel := backoffPolicy.Start(context.Background())
		defer cancel()
		for backoff.Continue(bo) {
			gorillaz.Log.Info("Listening to port " + port)
			err := broadcast.UdpToBroadcaster(network.UdpSource{
				HostPort:        ":" + port,
				MaxDatagramSize: maxDatagramSize,
			}, b)
			if err == nil {
				return
			} else {
				gorillaz.Log.Warn("Could not broadcast for port "+port, zap.Error(err))
			}
		}
	}()

	return udpBroadcaster{Broadcaster: b, port: port}
}

func getVmInstance(project, zone string) *vmInstance {
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
		return vm
	}
	panic("Should never happen")
}

func getPortsToListen(vm *vmInstance) ([]string, int) {
	bo, cancel := backoffPolicy.Start(context.Background())
	defer cancel()
	for backoff.Continue(bo) {
		ports, ok := vm.labels["broadcast_ports"]
		if ok {
			maxSize := 8192
			maxDatagramSize, ok := vm.labels["broadcast_max_datagram_size"]
			if ok {
				var err error
				maxSize, err = strconv.Atoi(maxDatagramSize)
				if err != nil {
					maxSize = 8192
				} else {
					gorillaz.Sugar.Infof("Overriding max datagram size to %i", maxSize)
				}
			}
			return strings.Split(ports, "_"), maxSize
		} else {
			otherLabels := make([]string, len(vm.labels))
			i := 0
			for l := range vm.labels {
				otherLabels[i] = l
				i++
			}
			gorillaz.Log.Info("broadcast_ports label not found", zap.Strings("otherLabels", otherLabels))
		}
	}
	panic("Should never happen")
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

func addAndStartNewInstances(currentInstances map[string]instanceHandle, newInstances instances, broadcasters []udpBroadcaster) {
	for vmName, vmInstance := range newInstances {
		ctx, cancel := context.WithCancel(context.Background())
		gorillaz.Sugar.Infof("Adding unicast publication for VM instance %s", vmName)
		for _, addr := range vmInstance.addresses {
			for _, b := range broadcasters {
				go func() {
					bo, cancel := backoffPolicy.Start(ctx)
					defer cancel()
					for backoff.Continue(bo) {
						port := b.port
						gorillaz.Sugar.Infof("unicast to %s:%s", addr, port)
						err := network.BroadcasterToUdp(ctx, b.Broadcaster, addr+":"+port)
						if err != nil {
							return
						}
						gorillaz.Log.Warn("Could not unicast", zap.String("address", addr), zap.String("port", port), zap.Error(err))
					}
				}()
			}
		}
		currentInstances[vmName] = instanceHandle{
			ips:    vmInstance.addresses,
			cancel: cancel,
		}
	}
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
