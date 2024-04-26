package sharedInterfaces

// dbAdd
type DbAddRequest struct {
	ACL    string            `json:"acl"`
	Values map[string]string `json:"values"`
}

type DbAddResponse struct {
	DidSucceed map[string]bool `json:"didSucceed"`
	Error      string          `json:"error"`
}

// dbGet

type DbGetRequest struct {
	DidSucceed bool     `json:"didSucceed"`
	Values     []string `json:"values"`
}

type DbGetResponse struct {
	Success bool              `json:"didSucceed"`
	Keys    map[string]string `json:"keys"`
	Error   string            `json:"error"`
}

// dbDelete

type DbDeleteRequest struct {
	Values []string `json:"values"`
}

type DbDeleteResponse struct {
	DidSucceed map[string]bool `json:"didSucceed"`
	Error      string          `json:"error"`
}

// setGlobalWrite

type DbSetGlobalWriteRequest struct {
	Value bool `json:"value"`
}

type DbSetGlobalWriteResponse struct {
	DidSucceed bool   `json:"didSucceed"`
	Error      string `json:"error"`
}

// dbGetGlobalWrite

type DbGetGlobalWriteResponse struct {
	GlobalWrite bool   `json:"globalWrite"`
	Error       string `json:"error"`
}

// dbRemoveOrigin
type DbRemoveOriginResponse struct {
	DidSucceed bool   `json:"didSucceed"`
	Error      string `json:"error"`
}
