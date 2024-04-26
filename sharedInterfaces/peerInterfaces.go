package sharedInterfaces

type Request struct {
	Action      string `json:"action"`
	ForceDomain string `json:"forceDomain"`
	Data        any    `json:"data"`
}

type ResBody struct {
	Code   int `json:"code,omitempty"`
	Status any `json:"status,omitempty"`
}

type EResp struct {
	DidSucceed bool   `json:"didSucceed"`
	Error      string `json:"error"`
}
