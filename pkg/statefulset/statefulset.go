package statefulset

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	v1 "k8s.io/api/core/v1"
)

// Name is the name of the plugin used in the plugin registry and configurations.
const Name = "StatefulSetScheduler"

// Labels holds the labels configuration for the Scheduler.
type Labels struct {
	Pod  []string `envconfig:"pod" required:"true" desc:"Labels for Pod to be considered by the StatefulSetScheduler (any of the list)"`
	Node string   `envconfig:"node" required:"true" desc:"Label to match for a Node to be considered suitable for scheduling a Pod"`
}

// Scheduler is a plugin that implements sorting based on the pod index and node's label match.
type Scheduler struct {
	Labels Labels
}

// NewScheduler initializes and returns a new Scheduler plugin.
func NewScheduler(labels Labels) (framework.Plugin, error) {
	return &Scheduler{
		Labels: labels,
	}, nil
}

// Ensure Scheduler implements the necessary interfaces.
var (
	_ framework.PreEnqueuePlugin = &Scheduler{}
	_ framework.PreFilterPlugin  = &Scheduler{}
	_ framework.FilterPlugin     = &Scheduler{}
)

// Name returns name of the plugin.
func (s *Scheduler) Name() string {
	return Name
}

// PreEnqueue checks if the pod should be considered for scheduling.
func (s *Scheduler) PreEnqueue(_ context.Context, pod *v1.Pod) *framework.Status {
	// Check if the Pod is owned by a StatefulSet.
	if !isOwnedByStatefulSet(pod) {
		msg := fmt.Sprintf("Pod %s is not owned by a StatefulSet", pod.Name)
		klog.V(1).InfoS(msg, "pod", pod.Name)
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, msg)
	}

	return nil
}

// isOwnedByStatefulSet checks if the given pod is owned by a StatefulSet.
func isOwnedByStatefulSet(pod *v1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "StatefulSet" {
			return true
		}
	}
	return false
}

// PreFilterExtensions returns nil as Scheduler does not have any prefilter extensions.
func (s *Scheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// PreFilter checks if a pod can be scheduled based on its labels.
func (s *Scheduler) PreFilter(_ context.Context, _ *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	// Check if the Pod has any of the required labels.
	if !hasAnyLabel(pod, s.Labels.Pod) {
		msg := fmt.Sprintf("Pod %s does not have any of the required labels: %v", pod.Name, s.Labels.Pod)
		klog.V(1).InfoS(msg, "pod", pod.Name)
		return nil, framework.NewStatus(framework.Unschedulable, msg)
	}

	klog.V(1).InfoS("Pod passed prefilter successfully", "pod", pod.Name)
	return nil, nil
}

// hasAnyLabel checks if the pod has any of the specified labels.
func hasAnyLabel(pod *v1.Pod, labelKeys []string) bool {
	for _, key := range labelKeys {
		if _, exists := pod.Labels[key]; exists {
			return true
		}
	}
	return false
}

// Filter checks if a node is suitable for scheduling the pod based on node labels and pod ordinal.
func (s *Scheduler) Filter(_ context.Context, _ *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()

	// Get the node label value as an integer.
	nodeLabelValue, err := getNodeLabelValue(node, s.Labels.Node)
	if err != nil {
		klog.V(1).InfoS("Filter failed", "node", node.Name, "error", err)
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	// Get the pod ordinal number.
	podOrdinal, err := getPodOrdinal(pod)
	if err != nil {
		klog.V(1).InfoS("Filter failed", "pod", pod.Name, "error", err)
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	// Check if the node label value matches the pod ordinal.
	if nodeLabelValue != podOrdinal {
		msg := fmt.Sprintf("Node %s is not suitable for pod %s placement", node.Name, pod.Name)
		klog.V(1).InfoS(msg, "node", node.Name, "pod", pod.Name)
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	klog.V(1).InfoS("Node passed filter successfully", "node", node.Name, "pod", pod.Name)
	return nil
}

// getNodeLabelValue retrieves and parses the node's label value as an integer.
func getNodeLabelValue(node *v1.Node, labelKey string) (int, error) {
	valueStr, exists := node.Labels[labelKey]
	if !exists {
		return 0, fmt.Errorf("node %s does not have label %q", node.Name, labelKey)
	}

	valueInt, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("node %s has invalid label %q value: %q, expected integer", node.Name, labelKey, valueStr)
	}

	return valueInt, nil
}

// getPodOrdinal extracts the ordinal number from the pod's name.
func getPodOrdinal(pod *v1.Pod) (int, error) {
	parts := strings.Split(pod.Name, "-")
	if len(parts) == 0 {
		return 0, fmt.Errorf("pod %s name format is invalid", pod.Name)
	}

	ordinalStr := parts[len(parts)-1]
	ordinal, err := strconv.Atoi(ordinalStr)
	if err != nil {
		return 0, fmt.Errorf("pod %s has invalid ordinal number", pod.Name)
	}

	return ordinal, nil
}
