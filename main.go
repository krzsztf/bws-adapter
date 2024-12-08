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
	if systemdRunDir, ok := os.LookupEnv("RUNTIME_DIRECTORY"); ok {
		return systemdRunDir
	}
	if xdgRunDir, ok := os.LookupEnv("XDG_RUNTIME_DIR"); ok {
		return xdgRunDir
	}
	return "/run"
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

func GetSecretManagerToken() (TokenResponse, error) {
	var bwsAccessToken, ok = os.LookupEnv("BWS_ACCESS_TOKEN")
	if !ok {
		return TokenResponse{}, errors.New("BWS access token not provided")
	}

	var sep = regexp.MustCompile("[.:]")
	var parts = sep.Split(bwsAccessToken, -1)
	if len(parts) != 4 {
		return TokenResponse{}, errors.New("Unexpected BWS access token format")
	}
	if parts[0] != "0" {
		return TokenResponse{}, errors.New("Unexpected BWS access token version")
	}

	log.Printf("Fetching BWS token for client_id=%s", parts[1])

	// TODO client_seccret in body/headers?
	// PostForms vs BasicAuth
	var params = url.Values{}
	params.Set("scope", "api.secrets")
	params.Set("client_id", parts[1])
	params.Set("client_secret", parts[2])
	params.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", bitwardenIdpTokenUrl, strings.NewReader(params.Encode()))
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Failed to build request: %s", err))
	}
	//req.SetBasicAuth(parts[1], parts[2])
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Request failed: %s", err))
	}
	// check status
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Failed reading response: %s", err))
	}

	var data TokenResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return TokenResponse{}, errors.New(fmt.Sprintf("Failed to parse response: %s", err))
	}
	return data, nil
}

func GetSecretManagerOrgId() (string, error) {
	accessToken, err := GetSecretManagerToken()
	if err != nil {
		return "", err
	}
	parsed, err := jws.ParseString(accessToken.AccessToken)
	if err != nil {
		return "", err
	}

	var payload map[string]interface{}
	err = json.Unmarshal(parsed.Payload(), &payload)
	if err != nil {
		return "", nil
	}

	orgId, ok := payload["organization"].(string)
	if !ok {
		return "", errors.New("Missing organization claim")
	}

	log.Printf("Extracted orgId=%s", orgId)
	return orgId, nil
}

func CreateBitwardenClient() (sdk.BitwardenClientInterface, error) {
	var bwsAccessToken, ok = os.LookupEnv("BWS_ACCESS_TOKEN")
	if !ok {
		return nil, errors.New("BWS access token not provided")
	}

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
	var address = fmt.Sprintf("%s/bws.socket", GetRuntimeDirectory())
	log.Printf("Listenting to %s", address)

	socket, err := net.Listen("unix", address)
	if err != nil {
		log.Fatalf("Error listening to socket: %s", err)
	}
	defer socket.Close()

	orgId, err := GetSecretManagerOrgId()
	if err != nil {
		log.Fatal(err)
	}
	client, err := CreateBitwardenClient()
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
