package request

type ModelFormatRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type PredictorRequest struct {
	ModelFormat        ModelFormatRequest `json:"modelFormat"`
	Runtime            string             `json:"runtime"`
	StorageURI         string             `json:"storageUri"`
	ServiceAccountName string             `json:"serviceAccountName"`
}

type CreateModelServiceRequest struct {
	Name      string            `json:"name"`
	Backend   string            `json:"backend,omitempty"`
	Image     string            `json:"image,omitempty"`
	Replicas  int32             `json:"replicas"`
	Port      int32             `json:"port,omitempty"`
	Predictor *PredictorRequest `json:"predictor,omitempty"`

	Exposure ExposureRequest `json:"exposure"`
	Storage  StorageRequest  `json:"storage"`
}

type UpdateModelServiceRequest struct {
	Backend   string            `json:"backend,omitempty"`
	Image     string            `json:"image,omitempty"`
	Replicas  int32             `json:"replicas"`
	Port      int32             `json:"port,omitempty"`
	Predictor *PredictorRequest `json:"predictor,omitempty"`

	Exposure ExposureRequest `json:"exposure"`
	Storage  StorageRequest  `json:"storage"`
}

type PatchModelServiceRequest struct {
	Backend   *string               `json:"backend,omitempty"`
	Image     *string               `json:"image,omitempty"`
	Replicas  *int32                `json:"replicas,omitempty"`
	Port      *int32                `json:"port,omitempty"`
	Predictor *PredictorRequest     `json:"predictor,omitempty"`
	Exposure  *PatchExposureRequest `json:"exposure,omitempty"`
	Storage   *PatchStorageRequest  `json:"storage,omitempty"`
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
