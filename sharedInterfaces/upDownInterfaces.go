package sharedInterfaces

//Upload

type UploadHeader struct {
	Filename   string
	Size       int64
	Domain     string
	Encryption bool
}

type UploadPercent struct {
	Percent int `json:"percent"`
}

type UploadSuccess struct {
	DidSucceed bool   `json:"didSucceed"`
	Hash       string `json:"hash"`
	FileName   string `json:"fileName"`
	BytesRead  int64  `json:"bytesRead"`
	Error      string `json:"error"`
}

//Download

type DownloadHeader struct {
	Filename     string
	ContinueFrom int64
	Domain       string
	Encryption   bool
}
type DownloadSuccess struct {
	DidSucceed bool   `json:"didSucceed"`
	FileName   string `json:"fileName"`
	Error      string `json:"error"`
}

type DownloadInitialMessage struct {
	Size int64 `json:"size"`
}
