package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bitwarden/sdk-go"
	"github.com/lestrrat-go/jwx/v2/jws"
)

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

func GetRuntimeDirectory() string {
	baseDir := "/run/bws"
	if systemdRunDir, ok := os.LookupEnv("RUNTIME_DIRECTORY"); ok {
		baseDir = systemdRunDir
	} else if xdgRunDir, ok := os.LookupEnv("XDG_RUNTIME_DIR"); ok {
		baseDir = xdgRunDir + "/bws"
	}
	return baseDir
}

const bitwardenIdpUrl = "https://identity.bitwarden.com"
const bitwardenIdpTokenUrl = bitwardenIdpUrl + "/connect/token"
const bitwardenIdpJwksUrl = bitwardenIdpUrl + "/.well-known/openid-configuration/jwks"
const bitwardenApiUrl = "https://api.bitwarden.com"

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

func GetBwsAccessToken() (string, error) {
	bwsAccessToken, ok := os.LookupEnv("BWS_ACCESS_TOKEN")

	if !ok {
		bwsAccessTokenFile, ok := os.LookupEnv("BWS_ACCESS_TOKEN_FILE")
		if !ok {
			return "", errors.New("BWS access token or token file not provided")
		}

		log.Printf("Reading token from %s", bwsAccessTokenFile)
		bwsAccessTokenData, err := os.ReadFile(bwsAccessTokenFile)
		if err != nil {
			return "", errors.New(fmt.Sprintf("Failed to read access token file: %s", err))
		}

		bwsAccessToken = string(bwsAccessTokenData)
	}

	return bwsAccessToken, nil
}

var tokenSeparator = regexp.MustCompile("[.:]")

func GetSecretManagerToken(bwsAccessToken string) (TokenResponse, error) {
	parts := tokenSeparator.Split(bwsAccessToken, -1)
	if len(parts) != 4 {
		return TokenResponse{}, errors.New(fmt.Sprintf("Unexpected BWS access token format: '%s'", bwsAccessToken))
	}
	if parts[0] != "0" {
		return TokenResponse{}, errors.New("Unexpected BWS access token version")
	}

	log.Printf("Fetching BWS token for client_id=%s", parts[1])

	var params = url.Values{}
	params.Set("scope", "api.secrets")
	params.Set("client_id", parts[1])
	params.Set("client_secret", parts[2])
	params.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", bitwardenIdpTokenUrl, strings.NewReader(params.Encode()))
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Failed to build request: %s", err))
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Request failed: %s", err))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Failed reading response: %s", err))
	}

	if resp.StatusCode != 200 {
		return TokenResponse{}, errors.New(fmt.Sprintf("Failed to obtain token: status=%d error=%s", resp.StatusCode, body))
	}

	var data TokenResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Failed to parse response: %s", err))
	}
	return data, nil
}

func GetSecretManagerOrgId(bwsAccessToken string) (string, error) {
	accessToken, err := GetSecretManagerToken(bwsAccessToken)
	if err != nil {
		return "", err
	}
	parsed, err := jws.ParseString(accessToken.AccessToken)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Failed to parse access token: %s, '%s'", err, accessToken.AccessToken))
	}

	var payload map[string]interface{}
	err = json.Unmarshal(parsed.Payload(), &payload)
	if err != nil {
		return "", err
	}

	orgId, ok := payload["organization"].(string)
	if !ok {
		return "", errors.New("Missing organization claim")
	}

	log.Printf("Extracted orgId=%s", orgId)
	return orgId, nil
}

func CreateBitwardenClient(bwsAccessToken string) (sdk.BitwardenClientInterface, error) {
	bitwardenClient, err := sdk.NewBitwardenClient(nil, nil)
	if err == nil {
		err = bitwardenClient.AccessTokenLogin(bwsAccessToken, nil)
	}
	return bitwardenClient, err
}

func FetchSecret(secretsClient sdk.SecretsInterface, orgId string, secretKey string) (*sdk.SecretResponse, error) {
	log.Printf("Fetching secrets in %s", orgId)
	secrets, err := secretsClient.List(orgId)
	if err != nil {
		return nil, err
	}

	for _, secret := range secrets.Data {
		if secret.Key == secretKey {
			log.Printf("Fetching secret %s by %s", secretKey, secret.ID)
			return secretsClient.Get(secret.ID)
		}
	}

	return nil, errors.New("Secret not found")
}

func main() {
	runtimeDir := GetRuntimeDirectory()
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		log.Fatalf("Failed to create runtime directory: %v", err)
	}

	var address = fmt.Sprintf("%s/bws.sock", runtimeDir)
	log.Printf("Listening to %s", address)

	// Remove existing socket file if it exists
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: could not remove existing socket file: %v", err)
	}

	socket, err := net.Listen("unix", address)
	if err != nil {
		log.Fatalf("Error listening to socket: %s", err)
	}
	defer socket.Close()

	bwsAccessToken, err := GetBwsAccessToken()
	if err != nil {
		log.Fatal(err)
	}

	orgId, err := GetSecretManagerOrgId(bwsAccessToken)
	if err != nil {
		log.Fatal(err)
	}
	client, err := CreateBitwardenClient(bwsAccessToken)
	if err != nil {
		log.Fatal(err)
	}
	for {
		func() {
			conn, err := socket.Accept()
			if err != nil {
				log.Fatalf("Error accepting connection: %s", err)
			}
			defer conn.Close()

			var peer = conn.RemoteAddr().String()
			var parts = strings.Split(peer, "/")
			if len(parts) != 4 {
				log.Fatalf("Unexpected peer name: %s", peer)
			}
			var secretName = parts[3]
			log.Printf("Got connection from %s for secret %s", peer, secretName)

			secret, err := FetchSecret(client.Secrets(), orgId, secretName)
			if err != nil {
				log.Print(err)
				return
			}

			n, err := conn.Write([]byte(secret.Value))
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Written %d bytes to conn", n)
		}()
	}
}
