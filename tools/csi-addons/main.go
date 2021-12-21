/*
Copyright 2021 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	endpoint    = "unix:///tmp/csi-addons.sock"
	stagingPath = "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/"
)

// command contains the parsed arguments that were passed while running the
// executable.
type command struct {
	endpoint         string
	stagingPath      string
	operation        string
	persistentVolume string
}

// cmd is the single instance of the command struct, used inside main().
var cmd = &command{}

func init() {
	flag.StringVar(&cmd.endpoint, "endpoint", endpoint, "CSI-Addons endpoint")
	flag.StringVar(&cmd.stagingPath, "stagingpath", stagingPath, "staging path")
	flag.StringVar(&cmd.operation, "operation", "", "csi-addons operation")
	flag.StringVar(&cmd.persistentVolume, "persistentvolume", "", "name of the PersistentVolume")

	// output to show when --help is passed
	flag.Usage = func() {
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintln(flag.CommandLine.Output(), "The following operations are supported:")
		for op := range operations {
			fmt.Fprintln(flag.CommandLine.Output(), " - "+op)
		}
		os.Exit(0)
	}

	flag.Parse()
}

func main() {
	op, found := operations[cmd.operation]
	if !found {
		fmt.Printf("ERROR: operation %q not found\n", cmd.operation)
		os.Exit(1)
	}

	op.Connect(cmd.endpoint)

	err := op.Init(cmd)
	if err != nil {
		err = fmt.Errorf("failed to initialize %q: %w", cmd.operation, err)
	} else {
		err = op.Execute()
		if err != nil {
			err = fmt.Errorf("failed to execute %q: %w", cmd.operation, err)
		}
	}

	op.Close()

	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
}

// getKubernetesClient returns a Clientset so that the Kubernetes API can be
// used. In case the Clientset can not be created, this function will panic as
// there will be no use of running the tool.
func getKubernetesClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

// getSecret get the secret details by name.
func getSecret(c *kubernetes.Clientset, ns, name string) (map[string]string, error) {
	secrets := make(map[string]string)

	secret, err := c.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	for k, v := range secret.Data {
		secrets[k] = string(v)
	}

	return secrets, nil
}

// operations contain a list of all available operations. Each operation should
// be added by calling registerOperation().
var operations = make(map[string]operation)

// operation is the interface that all operations should implement. The
// Connect() and Close() functions can be inherited from the grpcClient struct.
type operation interface {
	Connect(endpoint string)
	Close()

	Init(c *command) error
	Execute() error
}

// grpcClient provides standard Connect() and Close() functions that an
// operation needs to provide.
type grpcClient struct {
	Client *grpc.ClientConn
}

// Connect to the endpoint, or panic in case it fails.
func (g *grpcClient) Connect(endpoint string) {
	conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
	if err != nil {
		panic(fmt.Sprintf("failed to connect to %q: %v", endpoint, err))
	}

	g.Client = conn
}

// Close the connected grpc.ClientConn.
func (g *grpcClient) Close() {
	g.Client.Close()
}

// registerOperation adds a new operation struct to the operations map.
func registerOperation(name string, op operation) error {
	if _, ok := operations[name]; ok {
		return fmt.Errorf("operation %q is already registered", name)
	}

	operations[name] = op

	return nil
}
