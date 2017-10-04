// Copyright 2017 The Cacophony Project. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// NewAPI creates a CacophonyAPI instance and obtains a fresh JSON Web
// Token. If no password is given then the device is registered.
func NewAPI(serverURL, group, deviceName, password string) (*CacophonyAPI, error) {
	api := &CacophonyAPI{
		serverURL:  serverURL,
		group:      group,
		deviceName: deviceName,
		password:   password,
	}
	if password == "" {
		err := api.register()
		if err != nil {
			return nil, err
		}
	} else {
		err := api.newToken()
		if err != nil {
			return nil, err
		}
	}
	return api, nil
}

type CacophonyAPI struct {
	serverURL  string
	group      string
	deviceName string
	password   string
	token      string
}

func (api *CacophonyAPI) Password() string {
	return api.password
}

func (api *CacophonyAPI) register() error {
	if api.password != "" {
		return errors.New("already registered")
	}

	password := randString(20)
	payload, err := json.Marshal(map[string]string{
		"group":      api.group,
		"devicename": api.deviceName,
		"password":   password,
	})
	if err != nil {
		return err
	}
	postResp, err := http.Post(
		api.serverURL+"/api/v1/devices",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	defer postResp.Body.Close()

	var respData tokenResponse
	d := json.NewDecoder(postResp.Body)
	if err := d.Decode(&respData); err != nil {
		return fmt.Errorf("decode: %v", err)
	}
	if !respData.Success {
		return fmt.Errorf("registration failed: %v", respData.message())
	}

	api.password = password
	api.token = respData.Token
	return nil
}

func (api *CacophonyAPI) newToken() error {
	if api.password == "" {
		return errors.New("no password set")
	}
	payload, err := json.Marshal(map[string]string{
		"devicename": api.deviceName,
		"password":   api.password,
	})
	if err != nil {
		return err
	}
	postResp, err := http.Post(
		api.serverURL+"/authenticate_device",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	defer postResp.Body.Close()

	var resp tokenResponse
	d := json.NewDecoder(postResp.Body)
	if err := d.Decode(&resp); err != nil {
		return fmt.Errorf("decode: %v", err)
	}
	if !resp.Success {
		return fmt.Errorf("registration failed: %v", resp.message())
	}
	api.token = resp.Token
	return nil
}

type tokenResponse struct {
	Success  bool
	Messages []string
	Token    string
}

func (r *tokenResponse) message() string {
	if len(r.Messages) > 0 {
		return r.Messages[0]
	}
	return "unknown"
}
