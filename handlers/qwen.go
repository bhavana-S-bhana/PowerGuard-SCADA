package handlers

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type QwenRequest struct {
	Model    string        `json:"model"`
	Messages []QwenMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type QwenMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type QwenResponse struct {
	Message QwenMessage `json:"message"`
}

func askQwen(prompt string) (string, error) {

	requestBody := QwenRequest{
		Model: "qwen3:1.7b",
		Messages: []QwenMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Stream: false,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{
    Timeout: 120 * time.Second,
}

response, err := client.Post(
    "http://localhost:11434/api/chat",
    "application/json",
    bytes.NewBuffer(jsonData),
)
	if err != nil {
		return "", fmt.Errorf("could not connect to Ollama: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama returned status: %s", response.Status)
	}

	var result QwenResponse

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Message.Content, nil
}