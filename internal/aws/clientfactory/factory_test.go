package clientfactory

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/config"
)

func TestFactoryAssumeRoleUsesExternalIDAndCredentialCache(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "base-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("TEST_EXTERNAL_ID", "private-external-id")
	var stsCalls, ceCalls atomic.Int32
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
		ceCalls.Add(1)
		writer.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = io.WriteString(writer, `{"ResultsByTime":[]}`)
	}))
	defer server.Close()
	value := config.Default().AWS
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
	clients, err := factory.ForTarget(config.TargetConfig{Name: "payer", AccountID: "444455556666", AssumeRole: &config.AssumeRoleConfig{RoleARN: "arn:aws:iam::444455556666:role/exporter", ExternalIDEnv: "TEST_EXTERNAL_ID"}})
	if err != nil {
		t.Fatal(err)
	}
	input := &costexplorer.GetCostAndUsageInput{TimePeriod: &cetypes.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")}, Granularity: cetypes.GranularityDaily, Metrics: []string{"UnblendedCost"}}
	for range 2 {
		if _, err := clients.CostExplorer.GetCostAndUsage(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	if stsCalls.Load() != 1 || ceCalls.Load() != 2 {
		t.Fatalf("STS=%d CE=%d", stsCalls.Load(), ceCalls.Load())
	}
	body, _ := stsBody.Load().(string)
	for _, fragment := range []string{"RoleArn=arn%3Aaws%3Aiam%3A%3A444455556666%3Arole%2Fexporter", "ExternalId=private-external-id", "RoleSessionName=aws-cost-exporter-payer"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("AssumeRole body lacks %q: %s", fragment, body)
		}
	}
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
