package request

type CreateModelServiceRequest struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Replicas int32  `json:"replicas"`
	Port     int32  `json:"port"`

	Exposure ExposureRequest `json:"exposure"`
	Storage  StorageRequest  `json:"storage"`
}

type UpdateModelServiceRequest struct {
	Image    string `json:"image"`
	Replicas int32  `json:"replicas"`
	Port     int32  `json:"port"`

	Exposure ExposureRequest `json:"exposure"`
	Storage  StorageRequest  `json:"storage"`
}

type PatchModelServiceRequest struct {
	Image    *string               `json:"image,omitempty"`
	Replicas *int32                `json:"replicas,omitempty"`
	Port     *int32                `json:"port,omitempty"`
	Exposure *PatchExposureRequest `json:"exposure,omitempty"`
	Storage  *PatchStorageRequest  `json:"storage,omitempty"`
}

type PatchExposureRequest struct {
	Enabled    *bool   `json:"enabled,omitempty"`
	Hostname   *string `json:"hostname,omitempty"`
	PathPrefix *string `json:"pathPrefix,omitempty"`
}

type PatchStorageRequest struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	Size      *string `json:"size,omitempty"`
	MountPath *string `json:"mountPath,omitempty"`
}

type ExposureRequest struct {
	Enabled    bool   `json:"enabled"`
	Hostname   string `json:"hostname,omitempty"`
	PathPrefix string `json:"pathPrefix,omitempty"`
}

type StorageRequest struct {
	Enabled   bool   `json:"enabled"`
	Size      string `json:"size,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
}
