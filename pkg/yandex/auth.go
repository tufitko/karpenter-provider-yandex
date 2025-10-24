package yandex

import (
	"context"
	"encoding/json"
	"os"

	"github.com/pkg/errors"
	ycsdk "github.com/yandex-cloud/go-sdk"
	"github.com/yandex-cloud/go-sdk/iamkey"
)

const (
	IAMTokenEnv          = "YANDEX_IAM_TOKEN"
	OauthTokenEnv        = "YANDEX_OAUTH_TOKEN"
	ServiceAccountKeyEnv = "YANDEX_SERVICE_ACCOUNT_KEY"
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

	return ycsdk.InstanceServiceAccount(), nil
}
