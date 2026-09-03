package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func getHttpClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second}
}

func makeRequest(method, path string, body interface{}) (*http.Response, []byte, error) {
	url := fmt.Sprintf("%s%s", strings.TrimRight(endpoint, "/"), path)
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if projectID != "" {
		req.Header.Set("X-Project-ID", projectID)
	}

	resp, err := getHttpClient().Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	return resp, respBytes, err
}
