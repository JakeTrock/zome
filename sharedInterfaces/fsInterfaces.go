package sharedInterfaces

//PutObject

type PutObjectRequest struct {
	FileName     string            `json:"filename"`
	FileSize     int64             `json:"filesize"`
	Tagging      map[string]string `json:"tagging"`
	OverridePath string            `json:"overridePath"`
	ACL          string            `json:"acl"`  // acl of the metadata
	FACL         string            `json:"facl"` // acl of the file within metadata
	Encryption   bool              `json:"encryption"`
}

type PutObjectResponse struct {
	DidSucceed bool   `json:"didSucceed"`
	UploadId   string `json:"uploadId"` //the id which you will open a socket to in /upload/:id
	Error      string `json:"error"`
}

//GetObject

type GetObjectRequest struct {
	FileName     string `json:"fileName"`
	ContinueFrom int64  `json:"continueFrom"`
	ForceDomain  string `json:"forceDomain"`
	Encryption   bool   `json:"encryption"`
}

type GetObjectResponse struct {
	DidSucceed bool   `json:"didSucceed"`
	MetaData   string `json:"metadata"`
	DownloadId string `json:"downloadId"`
	Error      string `json:"error"`
}

//DeleteObject

type DeleteObjectRequest struct {
	FileName string `json:"fileName"`
}

type DeleteObjectResponse struct {
	DidSucceed bool   `json:"didSucceed"`
	Error      string `json:"error"`
}

//setGlobalFACL

type SetGlobalFACLRequest struct {
	Value bool `json:"value"`
}

type SetGlobalFACLResponse struct {
	DidSucceed bool   `json:"didSucceed"`
	Error      string `json:"error"`
}

//getGlobalFACL(empty req)

type GetGlobalFACLResponse struct {
	GlobalFsAccess bool   `json:"globalFsAccess"`
	Error          string `json:"error"`
}

//getDirectoryListing

type GetDirectoryListingRequest struct {
	Directory string `json:"directory"`
}

type GetDirectoryListingResponse struct {
	DidSucceed bool              `json:"didSucceed"`
	Files      map[string]string `json:"files"`
	Error      string            `json:"error"`
}

//removeObjectOrigin

type RemoveObjectOriginRequest struct {
	Directory string `json:"directory"`
}

type RemoveObjectOriginResponse struct {
	DidSucceed bool   `json:"didSucceed"`
	Error      string `json:"error"`
}
