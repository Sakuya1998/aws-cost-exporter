package costexplorer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

// TestNewSDKConfigUsesDefaultChainAndClientPolicy verifies the factory applies
// credentials, region, request timeout, and retry attempts.
func TestNewSDKConfigUsesDefaultChainAndClientPolicy(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	value := config.Default().AWS
	value.RequestTimeout = 125 * time.Millisecond
	value.Retry.MaxAttempts = 2
	sdkConfig, err := newSDKConfig(context.Background(), value)
	if err != nil {
		t.Fatalf("newSDKConfig() returned an unexpected error: %v", err)
	}

	credentials, err := sdkConfig.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials: %v", err)
	}
	if credentials.AccessKeyID != "test-access-key" || sdkConfig.Region != "us-east-1" {
		t.Fatalf("SDK config used credentials %q and region %q", credentials.AccessKeyID, sdkConfig.Region)
	}
	client, ok := sdkConfig.HTTPClient.(*awshttp.BuildableClient)
	if !ok || client.GetTimeout() != value.RequestTimeout {
		t.Fatalf("HTTP client = %#v, want timeout %v", sdkConfig.HTTPClient, value.RequestTimeout)
	}
	if attempts := sdkConfig.Retryer().MaxAttempts(); attempts != 2 {
		t.Fatalf("Retryer.MaxAttempts() = %d, want 2", attempts)
	}
}

// TestNewSDKConfigUsesNamedProfile verifies an explicit profile is resolved
// through the standard shared credentials chain.
func TestNewSDKConfigUsesNamedProfile(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	path := filepath.Join(t.TempDir(), "credentials")
	document := []byte("[finops]\naws_access_key_id = profile-access-key\naws_secret_access_key = profile-secret-key\n")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)

	value := config.Default().AWS
	value.Profile = "finops"
	sdkConfig, err := newSDKConfig(context.Background(), value)
	if err != nil {
		t.Fatalf("newSDKConfig() returned an unexpected error: %v", err)
	}
	credentials, err := sdkConfig.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials: %v", err)
	}
	if credentials.AccessKeyID != "profile-access-key" {
		t.Fatalf("profile AccessKeyID = %q, want profile-access-key", credentials.AccessKeyID)
	}
}

// TestNewAppliesExplicitEndpoint verifies test and proxy endpoints are scoped
// to the Cost Explorer service client.
func TestNewAppliesExplicitEndpoint(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")

	value := config.Default().AWS
	value.EndpointURL = "https://cost.example.test"
	client, err := New(context.Background(), value)
	if err != nil {
		t.Fatalf("New() returned an unexpected error: %v", err)
	}

	options := client.Options()
	if options.BaseEndpoint == nil || *options.BaseEndpoint != value.EndpointURL {
		t.Fatalf("BaseEndpoint = %v, want %q", options.BaseEndpoint, value.EndpointURL)
	}
}
