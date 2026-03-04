package yandex

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	iampb "github.com/yandex-cloud/go-genproto/yandex/cloud/iam/v1"
	"github.com/yandex-cloud/go-sdk/iamkey"
	"google.golang.org/protobuf/types/known/timestamppb"

	ycsdk "github.com/yandex-cloud/go-sdk"
)

const (
	IAMTokenEnv            = "YANDEX_IAM_TOKEN"
	OauthTokenEnv          = "YANDEX_OAUTH_TOKEN"
	ServiceAccountKeyEnv   = "YANDEX_SERVICE_ACCOUNT_KEY"
	SAIdEnv                = "YANDEX_SA_ID"
	SATokenFileEnv         = "YANDEX_SA_TOKEN_FILE"
	yandexTokenExchangeURL = "https://auth.yandex.cloud/oauth/token"
	oidcRefreshThreshold   = 5 * time.Minute
)

func buildSDK(ctx context.Context) (*ycsdk.SDK, error) {
	creds, err := credentialsFromEnv()
	if err != nil {
		return nil, err
	}

	return ycsdk.Build(ctx, ycsdk.Config{
		Credentials: creds,
	})
}

type tokenExchangeResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// https://yandex.cloud/ru/docs/iam/operations/wlif/setup-wlif#exchange-jwt-for-iam
func exchangeJWTForIAMToken(saID, jwt string) (accessToken string, expiresInSeconds int, err error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	form.Set("requested_token_type", "urn:ietf:params:oauth:token-type:access_token")
	form.Set("audience", saID)
	form.Set("subject_token", jwt)
	form.Set("subject_token_type", "urn:ietf:params:oauth:token-type:id_token")

	req, err := http.NewRequest(http.MethodPost, yandexTokenExchangeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, errors.Wrap(err, "create token exchange request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, errors.Wrap(err, "token exchange request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return "", 0, errors.Errorf("token exchange failed: %s: %s", resp.Status, buf.String())
	}

	var out tokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, errors.Wrap(err, "decode token exchange response")
	}
	if out.AccessToken == "" {
		return "", 0, errors.New("token exchange: empty access_token in response")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 43200
	}
	return out.AccessToken, out.ExpiresIn, nil
}

type oidcCredentials struct {
	saID   string
	getJWT func() string

	mu              sync.RWMutex
	cachedToken     string
	cachedExpiresAt time.Time
}

func (c *oidcCredentials) YandexCloudAPICredentials() {}

// IAMToken implements ycsdk.NonExchangeableCredentials for the old SDK.
func (c *oidcCredentials) IAMToken(ctx context.Context) (*iampb.CreateIamTokenResponse, error) {
	c.mu.RLock()
	if c.cachedToken != "" && c.cachedExpiresAt.After(time.Now().Add(oidcRefreshThreshold)) {
		tok, exp := c.cachedToken, c.cachedExpiresAt
		c.mu.RUnlock()
		return &iampb.CreateIamTokenResponse{IamToken: tok, ExpiresAt: timestamppb.New(exp)}, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	jwt := c.getJWT()
	if jwt == "" {
		return nil, errors.New("OIDC: JWT is not set")
	}
	iamToken, expiresIn, err := exchangeJWTForIAMToken(c.saID, jwt)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	c.cachedToken = iamToken
	c.cachedExpiresAt = expiresAt
	return &iampb.CreateIamTokenResponse{IamToken: iamToken, ExpiresAt: timestamppb.New(expiresAt)}, nil
}

func getJWTFromEnv() string {
	if path := os.Getenv(SATokenFileEnv); path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func credentialsFromEnv() (ycsdk.Credentials, error) {
	token := os.Getenv(IAMTokenEnv)
	if token != "" {
		return ycsdk.NewIAMTokenCredentials(token), nil
	}

	token = os.Getenv(OauthTokenEnv)
	if token != "" {
		return ycsdk.OAuthToken(token), nil
	}

	serviceAccountKeyPath := os.Getenv(ServiceAccountKeyEnv)
	if serviceAccountKeyPath != "" {
		var iamKey iamkey.Key

		raw, err := os.ReadFile(serviceAccountKeyPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read service account key from %s", serviceAccountKeyPath)
		}

		err = json.Unmarshal(raw, &iamKey)
		if err != nil {
			return nil, errors.Wrap(err, "malformed service account key json")
		}
		return ycsdk.ServiceAccountKey(&iamKey)
	}

	saID := os.Getenv(SAIdEnv)
	if saID != "" && os.Getenv(SATokenFileEnv) != "" {
		return &oidcCredentials{saID: saID, getJWT: getJWTFromEnv}, nil
	}

	return ycsdk.InstanceServiceAccount(), nil
}
