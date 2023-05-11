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

// Name of the plugin used in the plugin registry and configurations.
const Name = "StatefulSetScheduler"

// Labels holds the labels configuration for the STSScheduler.
type Labels struct {
	Pod  []string `envconfig:"pod" required:"true" desc:"Labels for Pod to be considered by the StatefulSetScheduler (any of the list)"` //nolint:lll
	Node string   `envconfig:"node" required:"true" desc:"Label to match for a Node to be considered suitable for scheduling a Pod"`     //nolint:lll
}

// STSScheduler is a plugin that implements sorting based on the pod index and node's label match.
type STSScheduler struct {
	Handle framework.Handle
	Labels
}

// NewSTSScheduler initializes and returns a new STSScheduler plugin.
func NewSTSScheduler(h framework.Handle, l Labels) (framework.Plugin, error) {
	return &STSScheduler{
		Handle: h,
		Labels: l,
	}, nil
}

var (
	// Let the type STSScheduler implement thePreFilterPlugin, FilterPlugin interface.
	_ framework.PreFilterPlugin = &STSScheduler{}
	_ framework.FilterPlugin    = &STSScheduler{}
)

// Name returns name of the plugin.
func (s *STSScheduler) Name() string {
	return Name
}

// PreFilterExtensions returns nil as the STSScheduler does not have a PreFilterExtensions.
func (s *STSScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// PreFilter is invoked at the prefilter extension point to check if a pod can be scheduled based on the
// specific requirements such as labels and the type of workload it belongs to.
func (s *STSScheduler) PreFilter(
	ctx context.Context,
	state *framework.CycleState,
	pod *v1.Pod,
) (*framework.PreFilterResult, *framework.Status) {
	// Check if the Pod is owned by a StatefulSet.
	if !isOwnedByStatefulSet(pod) {
		msg := fmt.Sprintf("Failed to pass prefilter. Pod %v is not owned by a StatefulSet", pod.Name)
		klog.V(1).InfoS(msg, "prefilter", "PreFilter")
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, msg)
	}

	podLabelsMatch := false
	for _, label := range s.Labels.Pod {
		if _, ok := pod.Labels[label]; !ok {
			continue
		}
		podLabelsMatch = true
	}
	if !podLabelsMatch {
		msg := fmt.Sprintf("Failed to pass pre filter. Pod %v has no labels %q", pod.Name, s.Labels.Pod)
		klog.V(1).InfoS(msg, "prefilter", "PreFilter")
		return nil, framework.NewStatus(framework.Unschedulable, msg)
	}

	klog.V(1).InfoS("Pod passed pre filter successfully", "prefilter", "PreFilter", "pod", pod.Name)
	return nil, framework.NewStatus(framework.Success, "Pass pre filter successfully")
}

// Filter is invoked at the filter extension point to check if a node satisfies the scheduling requirements
// imposed by the STSScheduler plugin.
func (s *STSScheduler) Filter(
	ctx context.Context,
	state *framework.CycleState,
	pod *v1.Pod,
	nodeInfo *framework.NodeInfo,
) *framework.Status {
	nodeName := nodeInfo.Node().Name

	nodeLabelValue, ok := nodeInfo.Node().Labels[s.Labels.Node]
	if !ok {
		msg := fmt.Sprintf("Failed to pass filter. Node %v has no label %q", nodeName, s.Labels.Node)
		klog.V(1).InfoS(msg, "filter", "Filter")
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	nodeLabelValueInt, err := strconv.Atoi(nodeLabelValue)
	if err != nil {
		msg := fmt.Sprintf(
			"Failed to pass filter. Node %v has wrong label %q value: %s. Expected integer.",
			nodeName,
			s.Labels.Node,
			nodeLabelValue,
		)
		klog.V(1).InfoS(msg, "filter", "Filter")
		return framework.NewStatus(framework.Error, msg)
	}

	splPodName := strings.Split(pod.Name, "-")
	podOrdinal, err := strconv.Atoi(splPodName[len(splPodName)-1])
	if err != nil {
		msg := fmt.Sprintf("Failed to pass filter. Pod %v has erroneous ordinal number", pod.Name)
		klog.V(1).InfoS(msg, "filter", "Filter")
		return framework.NewStatus(framework.Error, msg)
	}

	if podOrdinal != nodeLabelValueInt {
		msg := fmt.Sprintf("Failed to pass filter. Node %v not suitable for pod %v placement", nodeName, pod.Name)
		klog.V(1).InfoS(msg, "filter", "Filter")
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	klog.V(1).InfoS("Node passed filter successfully", "filter", "Filter", "node", nodeName)
	return framework.NewStatus(framework.Success, "Pass filter successfully")
}

// isOwnedByStatefulSet checks if the given Pod is owned by a StatefulSet.
func isOwnedByStatefulSet(pod *v1.Pod) bool {
	for _, ownerReference := range pod.OwnerReferences {
		if ownerReference.Kind == "StatefulSet" {
			return true
		}
	}

	return false
}
