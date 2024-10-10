package main

import (
	"context"
	"os"

	"github.com/kelseyhightower/envconfig"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-base/cli"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/grffio/k8s-sts-scheduler/pkg/statefulset"
)

// config holds the configuration for the scheduler.
type config struct {
	Labels statefulset.Labels `envconfig:"labels"`
}

func main() {
	var cnf config
	if err := envconfig.Process("sts-scheduler", &cnf); err != nil {
		klog.Fatalf("Invalid configuration: %s", err)
	}

	// Register custom plugins to the scheduler framework.
	command := app.NewSchedulerCommand(
		app.WithPlugin(statefulset.Name, func(
			_ context.Context,
			_ runtime.Object,
			_ framework.Handle,
		) (framework.Plugin, error) {
			return statefulset.NewScheduler(cnf.Labels)
		}),
	)

	code := cli.Run(command)
	os.Exit(code)
}
