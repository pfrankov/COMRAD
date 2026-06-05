package comrad

import "time"

type Condition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	Reason             string    `json:"reason" yaml:"reason"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime" yaml:"lastTransitionTime"`
}

type RuntimeSummary struct {
	APIVersion string               `json:"apiVersion" yaml:"apiVersion"`
	Kind       string               `json:"kind" yaml:"kind"`
	Items      []RuntimeSummaryItem `json:"items" yaml:"items"`
}

type RuntimeSummaryItem struct {
	Metadata ObjectMetadata       `json:"metadata" yaml:"metadata"`
	Spec     RuntimeSummarySpec   `json:"spec" yaml:"spec"`
	Status   RuntimeSummaryStatus `json:"status" yaml:"status"`
}

type ObjectMetadata struct {
	Name string `json:"name" yaml:"name"`
}

type RuntimeSummarySpec struct {
	Adapter       string               `json:"adapter" yaml:"adapter"`
	ModelFormats  []string             `json:"modelFormats" yaml:"modelFormats"`
	TaskKinds     []string             `json:"taskKinds" yaml:"taskKinds"`
	RuntimeBinary RuntimeBinarySummary `json:"runtimeBinary" yaml:"runtimeBinary"`
	ManagedArgs   []string             `json:"managedArgs" yaml:"managedArgs"`
}

type RuntimeBinarySummary struct {
	Source  string `json:"source" yaml:"source"`
	Command string `json:"command" yaml:"command"`
}

type RuntimeSummaryStatus struct {
	AvailableWorkers int `json:"availableWorkers" yaml:"availableWorkers"`
	ReadySlots       int `json:"readySlots" yaml:"readySlots"`
}

type CachePlan struct {
	ProfileRef       string              `json:"profileRef" yaml:"profileRef"`
	Artifacts        []string            `json:"artifacts" yaml:"artifacts"`
	RequireTags      []string            `json:"requireTags,omitempty" yaml:"requireTags,omitempty"`
	DesiredCopies    int                 `json:"desiredCopies" yaml:"desiredCopies"`
	ActualCopies     int                 `json:"actualCopies" yaml:"actualCopies"`
	StaleCopies      int                 `json:"staleCopies" yaml:"staleCopies"`
	EvictionsPending int                 `json:"evictionsPending" yaml:"evictionsPending"`
	Workers          []CacheWorkerStatus `json:"workers" yaml:"workers"`
	Conditions       []Condition         `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

type CacheWorkerStatus struct {
	NodeID   string              `json:"nodeId" yaml:"nodeId"`
	Cached   bool                `json:"cached" yaml:"cached"`
	Warm     bool                `json:"warm" yaml:"warm"`
	Active   bool                `json:"active" yaml:"active"`
	Eviction CacheEvictionStatus `json:"eviction" yaml:"eviction"`
	Intent   CacheIntentStatus   `json:"intent" yaml:"intent"`
}

type CacheEvictionStatus struct {
	Status    string    `json:"status" yaml:"status"`
	Reason    string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Failure   string    `json:"failure,omitempty" yaml:"failure,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

type CacheIntentStatus struct {
	Action    string    `json:"action,omitempty" yaml:"action,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}
