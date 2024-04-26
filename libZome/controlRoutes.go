package libzome

import (
	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/libp2p/go-libp2p/core/network"
)

// type ResBody struct {
// 	Code   int `json:"code,omitempty"`
// 	Status any `json:"status,omitempty"`
// }

type ZomeController struct {
	socket network.Stream
}

//TODO: so repetitive, how to refactor?

func (zc *ZomeController) DbAdd(putReq sharedInterfaces.DbAddRequest) (sharedInterfaces.DbAddResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-put", putReq)
	if err != nil {
		return sharedInterfaces.DbAddResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.DbAddResponse)
	if !okType {
		return sharedInterfaces.DbAddResponse{}, genericTypeError
	}
	return rspTyped, nil
}
func (zc *ZomeController) DbGet(putReq sharedInterfaces.DbGetRequest) (sharedInterfaces.DbGetResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-put", putReq)
	if err != nil {
		return sharedInterfaces.DbGetResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.DbGetResponse)
	if !okType {
		return sharedInterfaces.DbGetResponse{}, genericTypeError
	}
	return rspTyped, nil
}
func (zc *ZomeController) DbDelete(putReq sharedInterfaces.DbDeleteRequest) (sharedInterfaces.DbDeleteResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-put", putReq)
	if err != nil {
		return sharedInterfaces.DbDeleteResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.DbDeleteResponse)
	if !okType {
		return sharedInterfaces.DbDeleteResponse{}, genericTypeError
	}
	return rspTyped, nil
}
func (zc *ZomeController) DbSetGlobalWrite(putReq sharedInterfaces.DbSetGlobalWriteRequest) (sharedInterfaces.DbSetGlobalWriteResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-put", putReq)
	if err != nil {
		return sharedInterfaces.DbSetGlobalWriteResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.DbSetGlobalWriteResponse)
	if !okType {
		return sharedInterfaces.DbSetGlobalWriteResponse{}, genericTypeError
	}
	return rspTyped, nil
}
func (zc *ZomeController) DbGetGlobalWrite() (sharedInterfaces.DbGetGlobalWriteResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-put", []byte{})
	if err != nil {
		return sharedInterfaces.DbGetGlobalWriteResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.DbGetGlobalWriteResponse)
	if !okType {
		return sharedInterfaces.DbGetGlobalWriteResponse{}, genericTypeError
	}
	return rspTyped, nil
}
func (zc *ZomeController) DbRemoveOrigin() (sharedInterfaces.DbRemoveOriginResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-put", []byte{})
	if err != nil {
		return sharedInterfaces.DbRemoveOriginResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.DbRemoveOriginResponse)
	if !okType {
		return sharedInterfaces.DbRemoveOriginResponse{}, genericTypeError
	}
	return rspTyped, nil
}

func (zc *ZomeController) FsPut(putReq sharedInterfaces.PutObjectRequest) (sharedInterfaces.PutObjectResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-put", putReq)
	if err != nil {
		return sharedInterfaces.PutObjectResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.PutObjectResponse)
	if !okType {
		return sharedInterfaces.PutObjectResponse{}, genericTypeError
	}
	return rspTyped, nil
}

func (zc *ZomeController) FsGet(putReq sharedInterfaces.GetObjectRequest) (sharedInterfaces.GetObjectResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-get", putReq)
	if err != nil {
		return sharedInterfaces.GetObjectResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.GetObjectResponse)
	if !okType {
		return sharedInterfaces.GetObjectResponse{}, genericTypeError
	}
	return rspTyped, nil
}

func (zc *ZomeController) FsDelete(putReq sharedInterfaces.DeleteObjectRequest) (sharedInterfaces.DeleteObjectResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-delete", putReq)
	if err != nil {
		return sharedInterfaces.DeleteObjectResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.DeleteObjectResponse)
	if !okType {
		return sharedInterfaces.DeleteObjectResponse{}, genericTypeError
	}
	return rspTyped, nil
}

func (zc *ZomeController) FsSetGlobalWrite(putReq sharedInterfaces.SetGlobalFACLRequest) (sharedInterfaces.SetGlobalFACLResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-setGlobalWrite", putReq)
	if err != nil {
		return sharedInterfaces.SetGlobalFACLResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.SetGlobalFACLResponse)
	if !okType {
		return sharedInterfaces.SetGlobalFACLResponse{}, genericTypeError
	}
	return rspTyped, nil
}

func (zc *ZomeController) FsGetGlobalWrite() (sharedInterfaces.GetGlobalFACLResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-getGlobalWrite", []byte{})
	if err != nil {
		return sharedInterfaces.GetGlobalFACLResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.GetGlobalFACLResponse)
	if !okType {
		return sharedInterfaces.GetGlobalFACLResponse{}, genericTypeError
	}
	return rspTyped, nil
}

func (zc *ZomeController) FsRemoveOrigin(putReq sharedInterfaces.RemoveObjectOriginRequest) (sharedInterfaces.RemoveObjectOriginResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-removeOrigin", putReq)
	if err != nil {
		return sharedInterfaces.RemoveObjectOriginResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.RemoveObjectOriginResponse)
	if !okType {
		return sharedInterfaces.RemoveObjectOriginResponse{}, genericTypeError
	}
	return rspTyped, nil
}

func (zc *ZomeController) FsGetListing(putReq sharedInterfaces.GetDirectoryListingRequest) (sharedInterfaces.GetDirectoryListingResponse, error) {
	resp, err := genericRequest(zc.socket, "fs-getListing", putReq)
	if err != nil {
		return sharedInterfaces.GetDirectoryListingResponse{}, err
	}
	rspTyped, okType := resp.(sharedInterfaces.GetDirectoryListingResponse)
	if !okType {
		return sharedInterfaces.GetDirectoryListingResponse{}, genericTypeError
	}
	return rspTyped, nil
}
