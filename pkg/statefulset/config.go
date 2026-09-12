package statefulset

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// Args configures the StatefulSetOrdinal scheduler plugin.
type Args struct {
	// NodeOrdinalLabel is the node label whose value must equal the
	// StatefulSet pod ordinal.
	NodeOrdinalLabel string `json:"nodeOrdinalLabel"`
}

func (a Args) validate() error {
	if a.NodeOrdinalLabel == "" {
		return errors.New("nodeOrdinalLabel is required")
	}

	if problems := validation.IsQualifiedName(a.NodeOrdinalLabel); len(problems) != 0 {
		return fmt.Errorf(
			"nodeOrdinalLabel %q is not a valid Kubernetes label key: %s",
			a.NodeOrdinalLabel,
			strings.Join(problems, "; "),
		)
	}

	return nil
}
