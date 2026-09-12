package statefulset

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	fwk "k8s.io/kube-scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedutil "k8s.io/kubernetes/pkg/scheduler/util"
)

const (
	// Name is the scheduler framework registration name for the plugin.
	Name = "StatefulSetOrdinal"

	ordinalStateKey fwk.StateKey = Name + "/ordinal"

	podOrdinalSignKey = "k8s-sts-scheduler.grffio.github.com/pod-ordinal"
)

type plugin struct {
	nodeOrdinalLabelKey string
}

type preFilterState struct {
	ordinal int32
}

// Clone returns the immutable pre-filter state.
func (s *preFilterState) Clone() fwk.StateData {
	return s
}

var (
	_ fwk.PreFilterPlugin   = (*plugin)(nil)
	_ fwk.FilterPlugin      = (*plugin)(nil)
	_ fwk.EnqueueExtensions = (*plugin)(nil)
	_ fwk.SignPlugin        = (*plugin)(nil)
)

// New creates a StatefulSetOrdinal scheduler plugin.
func New(_ context.Context, config runtime.Object, _ fwk.Handle) (fwk.Plugin, error) {
	var args Args

	if err := frameworkruntime.DecodeInto(config, &args); err != nil {
		return nil, fmt.Errorf("%s: decode configuration: %w", Name, err)
	}

	if err := args.validate(); err != nil {
		return nil, fmt.Errorf("%s: invalid configuration: %w", Name, err)
	}

	return &plugin{
		nodeOrdinalLabelKey: args.NodeOrdinalLabel,
	}, nil
}

// Name returns the scheduler framework registration name.
func (*plugin) Name() string {
	return Name
}

// SignPod contributes the StatefulSet Pod ordinal to the scheduling signature.
//
// A signing failure only makes the Pod ineligible for batching optimization.
// PreFilter remains responsible for rejecting Pods with invalid ordinals.
func (*plugin) SignPod(_ context.Context, pod *corev1.Pod) ([]fwk.SignFragment, *fwk.Status) {
	if !isStatefulSetPod(pod) {
		return nil, nil
	}

	ordinal, err := podOrdinal(pod)
	if err != nil {
		return nil, fwk.NewStatus(fwk.Unschedulable, err.Error())
	}

	return []fwk.SignFragment{
		{
			Key:   podOrdinalSignKey,
			Value: ordinal,
		},
	}, nil
}

// PreFilter extracts the StatefulSet pod ordinal for use by Filter.
func (*plugin) PreFilter(_ context.Context, state fwk.CycleState, pod *corev1.Pod, _ []fwk.NodeInfo) (*fwk.PreFilterResult, *fwk.Status) {
	if !isStatefulSetPod(pod) {
		return nil, fwk.NewStatus(fwk.Skip)
	}

	ordinal, err := podOrdinal(pod)
	if err != nil {
		return nil, fwk.NewStatus(fwk.UnschedulableAndUnresolvable, err.Error())
	}

	if state == nil {
		return nil, fwk.AsStatus(errors.New("cycle state is nil"))
	}
	state.Write(ordinalStateKey, &preFilterState{ordinal: ordinal})

	return nil, nil
}

// PreFilterExtensions returns nil because the plugin has no incremental state.
func (*plugin) PreFilterExtensions() fwk.PreFilterExtensions {
	return nil
}

// Filter permits nodes whose configured ordinal matches the StatefulSet pod
// ordinal.
func (p *plugin) Filter(ctx context.Context, state fwk.CycleState, pod *corev1.Pod, nodeInfo fwk.NodeInfo) *fwk.Status {
	if err := context.Cause(ctx); err != nil {
		return fwk.NewStatus(fwk.UnschedulableAndUnresolvable, err.Error())
	}

	ordinal, err := readOrdinal(state)
	if err != nil {
		if !errors.Is(err, fwk.ErrNotFound) {
			return fwk.AsStatus(fmt.Errorf("%s: %w", Name, err))
		}

		// Filter can be configured without PreFilter, so derive the ordinal
		// directly from the Pod when CycleState does not contain it.
		if !isStatefulSetPod(pod) {
			return nil
		}

		ordinal, err = podOrdinal(pod)
		if err != nil {
			return fwk.NewStatus(fwk.UnschedulableAndUnresolvable, err.Error())
		}
	}

	if nodeInfo == nil || nodeInfo.Node() == nil {
		return fwk.AsStatus(
			errors.New("filter received NodeInfo without a Node"),
		)
	}

	node := nodeInfo.Node()

	nodeOrdinal, err := p.nodeOrdinal(node)
	if err != nil {
		return fwk.NewStatus(fwk.UnschedulableAndUnresolvable, err.Error())
	}

	if nodeOrdinal != ordinal {
		return fwk.NewStatus(
			fwk.UnschedulableAndUnresolvable,
			fmt.Sprintf("node %q has ordinal %d, pod requires ordinal %d", node.Name, nodeOrdinal, ordinal),
		)
	}

	return nil
}

// EventsToRegister returns cluster events that can make a rejected pod
// schedulable.
func (p *plugin) EventsToRegister(_ context.Context) ([]fwk.ClusterEventWithHint, error) {
	return []fwk.ClusterEventWithHint{
		{
			Event: fwk.ClusterEvent{
				Resource:   fwk.Node,
				ActionType: fwk.Add | fwk.UpdateNodeLabel,
			},
			QueueingHintFn: p.isSchedulableAfterNodeChange,
		},
		{
			Event: fwk.ClusterEvent{
				Resource:   fwk.TargetPod,
				ActionType: fwk.Update,
			},
			QueueingHintFn: p.isSchedulableAfterPodChange,
		},
	}, nil
}

func podOrdinal(pod *corev1.Pod) (int32, error) {
	ordinal, err := ordinalFromLabel(pod.Labels, appsv1.PodIndexLabel)
	if err != nil {
		return 0, fmt.Errorf("pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	return ordinal, nil
}

func (p *plugin) nodeOrdinal(node *corev1.Node) (int32, error) {
	ordinal, err := ordinalFromLabel(node.Labels, p.nodeOrdinalLabelKey)
	if err != nil {
		return 0, fmt.Errorf("node %q: %w", node.Name, err)
	}

	return ordinal, nil
}

func ordinalFromLabel(labels map[string]string, key string) (int32, error) {
	value, ok := labels[key]
	if !ok {
		return 0, fmt.Errorf("missing required ordinal label %q", key)
	}

	ordinal, err := parseOrdinal(value)
	if err != nil {
		return 0, fmt.Errorf("label %q has invalid ordinal %q: %w", key, value, err)
	}

	return ordinal, nil
}

func parseOrdinal(value string) (int32, error) {
	ordinal, err := strconv.ParseInt(value, 10, 32)
	if err != nil || ordinal < 0 {
		return 0, errors.New("ordinal must be a non-negative 32-bit decimal integer")
	}

	return int32(ordinal), nil
}

func isStatefulSetPod(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}

	owner := metav1.GetControllerOf(pod)
	return owner != nil &&
		owner.APIVersion == appsv1.SchemeGroupVersion.String() &&
		owner.Kind == "StatefulSet"
}

func readOrdinal(state fwk.CycleState) (int32, error) {
	rawState, err := state.Read(ordinalStateKey)
	if err != nil {
		return 0, fmt.Errorf("read ordinal from cycle state: %w", err)
	}

	preFilterState, ok := rawState.(*preFilterState)
	if !ok || preFilterState == nil {
		return 0, fmt.Errorf("invalid ordinal cycle state type %T", rawState)
	}

	return preFilterState.ordinal, nil
}

func (p *plugin) isSchedulableAfterNodeChange(_ klog.Logger, pod *corev1.Pod, oldObj, newObj any) (fwk.QueueingHint, error) {
	oldNode, newNode, err := schedutil.As[*corev1.Node](oldObj, newObj)
	if err != nil {
		return fwk.Queue, err
	}
	if newNode == nil {
		return fwk.Queue, errors.New("node event has nil new object")
	}

	if !isStatefulSetPod(pod) {
		return fwk.QueueSkip, nil
	}

	ordinal, err := podOrdinal(pod)
	if err != nil {
		//nolint:nilerr // A node change cannot fix an invalid Pod ordinal.
		return fwk.QueueSkip, nil
	}

	if !p.nodeMatchesOrdinal(newNode, ordinal) {
		return fwk.QueueSkip, nil
	}

	if oldNode == nil {
		return fwk.Queue, nil
	}

	if p.nodeMatchesOrdinal(oldNode, ordinal) {
		return fwk.QueueSkip, nil
	}

	return fwk.Queue, nil
}

func (*plugin) isSchedulableAfterPodChange(_ klog.Logger, _ *corev1.Pod, oldObj, newObj any) (fwk.QueueingHint, error) {
	oldPod, newPod, err := schedutil.As[*corev1.Pod](oldObj, newObj)
	if err != nil {
		return fwk.Queue, err
	}
	if oldPod == nil || newPod == nil {
		return fwk.Queue, errors.New("target Pod update requires old and new objects")
	}

	oldIsStatefulSet := isStatefulSetPod(oldPod)
	newIsStatefulSet := isStatefulSetPod(newPod)

	if oldIsStatefulSet && !newIsStatefulSet {
		return fwk.Queue, nil
	}

	if !newIsStatefulSet {
		return fwk.QueueSkip, nil
	}

	newOrdinal, err := podOrdinal(newPod)
	if err != nil {
		//nolint:nilerr // An invalid ordinal intentionally maps to QueueSkip.
		return fwk.QueueSkip, nil
	}

	if !oldIsStatefulSet {
		return fwk.Queue, nil
	}

	oldOrdinal, err := podOrdinal(oldPod)
	if err != nil || oldOrdinal != newOrdinal {
		//nolint:nilerr // Invalid-to-valid and ordinal changes require requeueing.
		return fwk.Queue, nil
	}

	return fwk.QueueSkip, nil
}

func (p *plugin) nodeMatchesOrdinal(node *corev1.Node, ordinal int32) bool {
	nodeOrdinal, err := p.nodeOrdinal(node)
	return err == nil && nodeOrdinal == ordinal
}
