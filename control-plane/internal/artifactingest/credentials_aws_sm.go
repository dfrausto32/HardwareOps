package artifactingest

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type awsSecretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

var loadAWSConfig = config.LoadDefaultConfig
var newAWSSecretsManagerClient = func(cfg aws.Config) awsSecretsManagerClient {
	return secretsmanager.NewFromConfig(cfg)
}

func LoadStaticCredentialsFromAWSSecretManager(ctx context.Context, secretID, region string) (map[string]map[string]string, error) {
	secretID = strings.TrimSpace(secretID)
	region = strings.TrimSpace(region)
	if secretID == "" {
		return nil, nil
	}

	loadOpts := []func(*config.LoadOptions) error{}
	if region != "" {
		loadOpts = append(loadOpts, config.WithRegion(region))
	}
	awsCfg, err := loadAWSConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("aws config load: %w", err)
	}
	client := newAWSSecretsManagerClient(awsCfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("aws secretsmanager get-secret-value: %w", err)
	}

	raw := strings.TrimSpace(valueOrEmpty(out.SecretString))
	if raw == "" && len(out.SecretBinary) > 0 {
		raw = strings.TrimSpace(string(out.SecretBinary))
	}
	if raw == "" {
		return nil, fmt.Errorf("secret %q has empty SecretString", secretID)
	}
	return parseStaticCredentialsRaw(raw)
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
