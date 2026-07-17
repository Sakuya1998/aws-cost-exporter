package clientfactory

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

func TestFactoryAssumeRoleUsesExternalIDAndCredentialCache(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "base-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("TEST_EXTERNAL_ID", "private-external-id")
	var stsCalls, identityCalls, ceCalls atomic.Int32
	var stsBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if strings.Contains(string(body), "Action=AssumeRole") {
			stsCalls.Add(1)
			stsBody.Store(string(body))
			writer.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(writer, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>assumed-access</AccessKeyId><SecretAccessKey>assumed-secret</SecretAccessKey><SessionToken>assumed-token</SessionToken><Expiration>2030-01-01T00:00:00Z</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::444455556666:assumed-role/exporter/session</Arn><AssumedRoleId>id:session</AssumedRoleId></AssumedRoleUser></AssumeRoleResult><ResponseMetadata><RequestId>request-id</RequestId></ResponseMetadata></AssumeRoleResponse>`)
			return
		}
		if strings.Contains(string(body), "Action=GetCallerIdentity") {
			identityCalls.Add(1)
			writer.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(writer, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Account>444455556666</Account><Arn>arn:aws:sts::444455556666:assumed-role/exporter/session</Arn><UserId>id:session</UserId></GetCallerIdentityResult><ResponseMetadata><RequestId>request-id</RequestId></ResponseMetadata></GetCallerIdentityResponse>`)
			return
		}
		ceCalls.Add(1)
		writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = io.WriteString(writer, `{"ResultsByTime":[]}`)
	}))
	defer server.Close()
	value := config.Default().AWS
	value.Credentials.Sources = map[string]config.CredentialSourceConfig{"runtime": {Type: config.CredentialSourceDefaultChain}}
	value.Endpoints.STS = server.URL
	value.Endpoints.CostExplorer = server.URL
	value.RequestTimeout = time.Second
	value.Retry.MaxAttempts = 1
	value.RateLimit.GlobalRequestsPerSecond = 10
	value.RateLimit.GlobalBurst = 5
	value.RateLimit.TargetRequestsPerSecond = 10
	value.RateLimit.TargetBurst = 5
	factory, err := New(context.Background(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	clients, err := factory.ForTarget(config.TargetConfig{Name: "payer", AccountID: "444455556666", Credentials: config.TargetCredentialsConfig{Source: "runtime", AssumeRole: &config.AssumeRoleConfig{RoleARN: "arn:aws:iam::444455556666:role/exporter", ExternalIDEnv: "TEST_EXTERNAL_ID"}}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := clients.Verifier.Verify(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	input := &costexplorer.GetCostAndUsageInput{TimePeriod: &cetypes.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")}, Granularity: cetypes.GranularityDaily, Metrics: []string{"UnblendedCost"}}
	for range 2 {
		if _, err := clients.CostExplorer.GetCostAndUsage(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	if stsCalls.Load() != 1 || identityCalls.Load() != 1 || ceCalls.Load() != 2 {
		t.Fatalf("AssumeRole=%d GetCallerIdentity=%d CE=%d", stsCalls.Load(), identityCalls.Load(), ceCalls.Load())
	}
	body, _ := stsBody.Load().(string)
	for _, fragment := range []string{"RoleArn=arn%3Aaws%3Aiam%3A%3A444455556666%3Arole%2Fexporter", "ExternalId=private-external-id", "RoleSessionName=aws-cost-exporter-payer"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("AssumeRole body lacks %q: %s", fragment, body)
		}
	}
}

func TestFactoryLoadsIndependentProfileAndStaticSources(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "environment-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "environment-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("LEGACY_ACCESS", "legacy-access")
	t.Setenv("LEGACY_SECRET", "legacy-secret")
	path := filepath.Join(t.TempDir(), "credentials")
	document := []byte("[account-a]\naws_access_key_id = profile-a-access\naws_secret_access_key = profile-a-secret\n[account-b]\naws_access_key_id = profile-b-access\naws_secret_access_key = profile-b-secret\n")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	value := config.Default().AWS
	value.Credentials.Sources = map[string]config.CredentialSourceConfig{
		"account-a": {Type: config.CredentialSourceProfile, Profile: "account-a"},
		"account-b": {Type: config.CredentialSourceProfile, Profile: "account-b"},
		"legacy":    {Type: config.CredentialSourceStaticEnv, AccessKeyIDEnv: "LEGACY_ACCESS", SecretAccessKeyEnv: "LEGACY_SECRET"},
	}
	factory, err := New(context.Background(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ source, account, access string }{
		{"account-a", "111122223333", "profile-a-access"},
		{"account-b", "444455556666", "profile-b-access"},
		{"legacy", "777788889999", "legacy-access"},
	} {
		clients, err := factory.ForTarget(config.TargetConfig{Name: test.source, AccountID: test.account, Credentials: config.TargetCredentialsConfig{Source: test.source}})
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := clients.CostExplorer.Options().Credentials.Retrieve(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if resolved.AccessKeyID != test.access {
			t.Fatalf("source %s AccessKeyID=%q, want %q", test.source, resolved.AccessKeyID, test.access)
		}
	}
}

func TestFactoryKeepsUnavailableProfileTargetIsolated(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "missing-credentials"))
	value := config.Default().AWS
	value.Credentials.Sources = map[string]config.CredentialSourceConfig{
		"missing": {Type: config.CredentialSourceProfile, Profile: "does-not-exist"},
	}
	factory, err := New(context.Background(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := factory.ValidateSources(); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("ValidateSources()=%v", err)
	}
	clients, err := factory.ForTarget(config.TargetConfig{Name: "optional", AccountID: "111122223333", Credentials: config.TargetCredentialsConfig{Source: "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	verifyErr := clients.Verifier.Verify(context.Background())
	safe, ok := verifyErr.(interface{ SafeKind() string })
	if verifyErr == nil || !ok || safe.SafeKind() != "validation" || strings.Contains(verifyErr.Error(), "does-not-exist") {
		t.Fatalf("Verify()=%v", verifyErr)
	}
}

func TestTargetVerifierSingleFlightAndMismatch(t *testing.T) {
	api := &identityStub{account: "111122223333"}
	verifier := &targetVerifier{client: api, target: "payer", accountID: "111122223333", failureTTL: time.Second, successTTL: time.Hour}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := verifier.Verify(context.Background()); err != nil {
				t.Errorf("Verify()=%v", err)
			}
		}()
	}
	group.Wait()
	if api.calls.Load() != 1 {
		t.Fatalf("GetCallerIdentity calls=%d, want 1", api.calls.Load())
	}
	verifier.mu.Lock()
	verifier.verifiedUntil = time.Now().Add(-time.Second)
	verifier.mu.Unlock()
	if err := verifier.Verify(context.Background()); err != nil || api.calls.Load() != 2 {
		t.Fatalf("expired Verify()=%v calls=%d, want revalidation", err, api.calls.Load())
	}
	mismatch := &targetVerifier{client: api, target: "other", accountID: "999900001111", failureTTL: time.Second}
	err := mismatch.Verify(context.Background())
	safe, ok := err.(interface{ SafeKind() string })
	if err == nil || !strings.Contains(err.Error(), "does not match") || !ok || safe.SafeKind() != "validation" {
		t.Fatalf("mismatch Verify()=%v", err)
	}
}

type identityStub struct {
	account string
	calls   atomic.Int32
}

func (stub *identityStub) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	stub.calls.Add(1)
	return &sts.GetCallerIdentityOutput{Account: aws.String(stub.account)}, nil
}

func TestSessionNameUsesConfiguredOrBoundedDefault(t *testing.T) {
	if got := sessionName("payer", "custom-session"); got != "custom-session" {
		t.Fatalf("configured=%q", got)
	}
	if got := sessionName("abcdefghijklmnopqrstuvwxyz123456", ""); got != "aws-cost-exporter-abcdefghijklmnopqrstuvwxyz123456" {
		t.Fatalf("default=%q", got)
	}
	if got := sessionName("abcdefghijklmnopqrstuvwxyz123456", ""); len(got) > 64 {
		t.Fatalf("session length=%d", len(got))
	}
}
