package main

import (
	"encoding/json"
)

//TODO: testme

func (a *App) getPeers(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Request
		Data struct {
			Value bool `json:"value"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
	}

}
