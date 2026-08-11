package response

type ModelServiceSummary struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Replicas int32  `json:"replicas"`
	State    string `json:"state"`
	Hostname string `json:"hostname,omitempty"`
}

type ModelServiceListResponse struct {
	Items []ModelServiceSummary `json:"items"`
	Count int                   `json:"count"`
}

type ModelServiceExposure struct {
	Enabled    bool   `json:"enabled"`
	Hostname   string `json:"hostname,omitempty"`
	PathPrefix string `json:"pathPrefix,omitempty"`
}

type ModelServiceStorage struct {
	Enabled   bool   `json:"enabled"`
	Size      string `json:"size,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
}

type ModelServiceResponse struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`

	Name string `json:"name"`

	Image    string `json:"image"`
	Replicas int32  `json:"replicas"`
	Port     int32  `json:"port"`

	Exposure ModelServiceExposure `json:"exposure"`
	Storage  ModelServiceStorage  `json:"storage"`

	State      string `json:"state"`
	Generation int64  `json:"generation"`
}

type ModelServiceCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type ModelServiceStatusResponse struct {
	Name               string                  `json:"name"`
	State              string                  `json:"state"`
	ObservedGeneration int64                   `json:"observedGeneration"`
	DesiredReplicas    int32                   `json:"desiredReplicas"`
	ReadyReplicas      int32                   `json:"readyReplicas"`
	Endpoint           string                  `json:"endpoint,omitempty"`
	Conditions         []ModelServiceCondition `json:"conditions"`
}
