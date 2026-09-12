package main

import (
	"os"

	"github.com/grffio/k8s-sts-scheduler/pkg/statefulset"

	"k8s.io/component-base/cli"
	_ "k8s.io/component-base/metrics/prometheus/clientgo"
	_ "k8s.io/component-base/metrics/prometheus/version"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func main() {
	command := app.NewSchedulerCommand(
		app.WithPlugin(statefulset.Name, statefulset.New),
	)

	os.Exit(cli.Run(command))
}
