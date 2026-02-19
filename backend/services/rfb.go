package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// RFBClient wraps communication with the Receita Federal CBS API.
type RFBClient struct {
	httpClient *http.Client
	baseURL    string // e.g. https://api.receitafederal.gov.br
	tokenURL   string // e.g. https://api.receitafederal.gov.br/token
	webhookURL string // e.g. https://fbtax.cloud/api/rfb/webhook
}

// RFB API response types
type RFBTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type RFBApuracaoResponse struct {
	Tiquete       string `json:"tiquete"`
	CodigoErro    string `json:"codigoErro"`
	MensagemErro  string `json:"mensagemErro"`
}

// NewRFBClient creates a new RFB API client from environment variables.
func NewRFBClient() *RFBClient {
	baseURL := os.Getenv("RFB_API_URL")
	if baseURL == "" {
		baseURL = "https://api.receitafederal.gov.br"
	}

	tokenURL := os.Getenv("RFB_TOKEN_URL")
	if tokenURL == "" {
		tokenURL = baseURL + "/token"
	}

	webhookURL := os.Getenv("RFB_WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = "https://fbtax.cloud/api/rfb/webhook"
	}

	return &RFBClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
		tokenURL:   tokenURL,
		webhookURL: webhookURL,
	}
}

// GetToken obtains an OAuth2 access token using client_credentials grant.
func (c *RFBClient) GetToken(clientID, clientSecret string) (string, error) {
	log.Printf("[RFB] Requesting OAuth2 token from %s", c.tokenURL)

	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", c.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[RFB] Token error (HTTP %d): %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("token request returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp RFBTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access_token in response")
	}

	log.Printf("[RFB] Token obtained successfully (expires_in: %d)", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, nil
}

// SolicitarApuracao sends a CBS assessment request to the RFB API.
// cnpjBase must be 8 digits (company root CNPJ).
// Returns the tiquete (ticket) for later download.
func (c *RFBClient) SolicitarApuracao(token, cnpjBase string) (string, error) {
	endpoint := fmt.Sprintf("%s/rtc/apuracao-cbs/v1/%s", c.baseURL, cnpjBase)
	log.Printf("[RFB] Requesting CBS assessment: POST %s (webhook: %s)", endpoint, c.webhookURL)

	payload := map[string]string{
		"urlRetorno": c.webhookURL,
	}
	payloadJSON, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return "", fmt.Errorf("failed to create assessment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("assessment request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[RFB] Assessment response (HTTP %d): %s", resp.StatusCode, string(body))

	var apuracaoResp RFBApuracaoResponse
	if err := json.Unmarshal(body, &apuracaoResp); err != nil {
		return "", fmt.Errorf("failed to parse assessment response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		errMsg := apuracaoResp.MensagemErro
		if errMsg == "" {
			errMsg = string(body)
		}
		return "", fmt.Errorf("assessment returned HTTP %d: [%s] %s", resp.StatusCode, apuracaoResp.CodigoErro, errMsg)
	}

	if apuracaoResp.Tiquete == "" {
		return "", fmt.Errorf("empty tiquete in response")
	}

	log.Printf("[RFB] Assessment requested successfully, tiquete: %s", apuracaoResp.Tiquete)
	return apuracaoResp.Tiquete, nil
}

// DownloadArquivo downloads the CBS assessment JSON file using the ticket.
// Returns the raw JSON bytes. Note: each ticket can only be downloaded ONCE.
func (c *RFBClient) DownloadArquivo(token, tiquete string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/rtc/download/v1/%s", c.baseURL, tiquete)
	log.Printf("[RFB] Downloading assessment file: GET %s", endpoint)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read download response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[RFB] Download error (HTTP %d): %s", resp.StatusCode, string(body))
		switch resp.StatusCode {
		case http.StatusForbidden:
			return nil, fmt.Errorf("CNPJ do consumidor não corresponde ao CNPJ da solicitação (HTTP 403)")
		case http.StatusNotFound:
			return nil, fmt.Errorf("arquivo não encontrado ou tíquete inválido (HTTP 404)")
		default:
			return nil, fmt.Errorf("download returned HTTP %d: %s", resp.StatusCode, string(body))
		}
	}

	log.Printf("[RFB] Download completed successfully (%d bytes)", len(body))
	return body, nil
}
