// Package clientfactory constructs isolated AWS clients for explicit targets.
package clientfactory

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
	appconfig "github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

// Clients contains the service clients belonging to one target credential boundary.
type Clients struct {
	CostExplorer  *costexplorer.Client
	Athena        *athena.Client
	Organizations *organizations.Client
	Budgets       *budgets.Client
	Retryer       func(string) aws.Retryer
	Verifier      Verifier
}

// Verifier confirms that final target credentials belong to the configured account.
type Verifier interface{ Verify(context.Context) error }

// Factory loads named credential sources once and creates target-scoped clients.
type Factory struct {
	sources      map[string]aws.Config
	sourceErrors map[string]error
	config       appconfig.AWSConfig
	global       awscommon.Limiter
	observer     awscommon.Observer
}

// New loads every unique credential source once.
func New(ctx context.Context, value appconfig.AWSConfig, observer awscommon.Observer) (*Factory, error) {
	httpClient := awshttp.NewBuildableClient().WithTimeout(value.RequestTimeout)
	sources := make(map[string]aws.Config, len(value.Credentials.Sources))
	sourceErrors := make(map[string]error)
	commonOptions := func() []func(*awsconfig.LoadOptions) error {
		return []func(*awsconfig.LoadOptions) error{
			awsconfig.WithRegion(value.Region), awsconfig.WithHTTPClient(httpClient),
			awsconfig.WithRetryer(func() aws.Retryer { return awscommon.NewRetryer(value.Retry) }),
		}
	}
	fallback := aws.Config{
		Region:      value.Region,
		HTTPClient:  httpClient,
		Credentials: aws.AnonymousCredentials{},
		Retryer:     func() aws.Retryer { return awscommon.NewRetryer(value.Retry) },
	}
	for name, source := range value.Credentials.Sources {
		options := commonOptions()
		switch source.Type {
		case appconfig.CredentialSourceProfile:
			options = append(options, awsconfig.WithSharedConfigProfile(source.Profile))
		case appconfig.CredentialSourceStaticEnv:
			provider := credentials.NewStaticCredentialsProvider(
				os.Getenv(source.AccessKeyIDEnv), os.Getenv(source.SecretAccessKeyEnv), os.Getenv(source.SessionTokenEnv),
			)
			base := fallback.Copy()
			base.Credentials = aws.NewCredentialsCache(provider)
			sources[name] = base
			continue
		case appconfig.CredentialSourceDefaultChain:
		default:
			sources[name] = fallback.Copy()
			sourceErrors[name] = errors.New("unsupported credential source type")
			continue
		}
		base, err := awsconfig.LoadDefaultConfig(ctx, options...)
		if err != nil {
			sources[name] = fallback.Copy()
			sourceErrors[name] = err
			continue
		}
		sources[name] = base
	}
	return &Factory{
		sources: sources, sourceErrors: sourceErrors, config: value,
		global:   awscommon.NewLimiter(value.RateLimit.GlobalRequestsPerSecond, value.RateLimit.GlobalBurst),
		observer: observer,
	}, nil
}

// ValidateSources reports credential-source construction errors without making AWS requests.
func (factory *Factory) ValidateSources() error {
	if factory == nil {
		return errors.New("invalid AWS client factory")
	}
	for name := range factory.config.Credentials.Sources {
		if err := factory.sourceErrors[name]; err != nil {
			return fmt.Errorf("load AWS credential source %s: %w", name, err)
		}
	}
	return nil
}

// ForTarget constructs clients without making a network request.
func (factory *Factory) ForTarget(target appconfig.TargetConfig) (Clients, error) {
	if factory == nil || target.Name == "" {
		return Clients{}, errors.New("invalid AWS client factory target")
	}
	base, exists := factory.sources[target.Credentials.Source]
	if !exists {
		return Clients{}, errors.New("target references unknown AWS credential source")
	}
	targetID := identity.TargetID(target.Name)
	sourceErr := factory.sourceErrors[target.Credentials.Source]
	limiter := awscommon.DualLimiter{
		Global: factory.global,
		Target: awscommon.NewLimiter(factory.config.RateLimit.TargetRequestsPerSecond, factory.config.RateLimit.TargetBurst),
	}
	retryers := make(map[string]aws.Retryer, 16)
	for _, operation := range []string{
		awscommon.OperationAssumeRole, awscommon.OperationGetCallerIdentity, awscommon.OperationGetCostAndUsage,
		awscommon.OperationGetCostForecast, awscommon.OperationListAccounts,
		awscommon.OperationDescribeOrganization, awscommon.OperationDescribeBudgets,
		awscommon.OperationGetSavingsPlansUtilization, awscommon.OperationGetSavingsPlansCoverage,
		awscommon.OperationGetReservationUtilization, awscommon.OperationGetReservationCoverage,
		awscommon.OperationGetAnomalies, awscommon.OperationStartQueryExecution,
		awscommon.OperationGetQueryExecution, awscommon.OperationGetQueryResults,
	} {
		retryers[operation] = awscommon.WrapRetryer(base.Retryer(), targetID, operation, limiter, factory.observer)
	}
	retryerFor := func(operation string) aws.Retryer { return retryers[operation] }
	sdkConfig := base.Copy()
	if sourceErr == nil && target.Credentials.AssumeRole != nil {
		role := target.Credentials.AssumeRole
		stsClient := sts.NewFromConfig(base, func(options *sts.Options) {
			options.Retryer = retryerFor(awscommon.OperationAssumeRole)
			if endpoint := strings.TrimSpace(factory.config.Endpoints.STS); endpoint != "" {
				options.BaseEndpoint = aws.String(endpoint)
			}
		})
		provider := stscreds.NewAssumeRoleProvider(observedSTS{client: stsClient, target: targetID, observer: factory.observer}, role.RoleARN, func(options *stscreds.AssumeRoleOptions) {
			externalID := os.Getenv(role.ExternalIDEnv)
			options.ExternalID = aws.String(externalID)
			options.RoleSessionName = sessionName(targetID, role.SessionName)
		})
		sdkConfig.Credentials = aws.NewCredentialsCache(provider)
	}
	clients := Clients{}
	clients.Retryer = retryerFor
	identityClient := sts.NewFromConfig(sdkConfig, func(options *sts.Options) {
		options.Retryer = retryerFor(awscommon.OperationGetCallerIdentity)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.STS); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	if sourceErr != nil {
		clients.Verifier = unavailableVerifier{cause: sourceErr}
	} else {
		clients.Verifier = &targetVerifier{
			client: identityClient, target: targetID, accountID: target.AccountID,
			credentials: sdkConfig.Credentials,
			observer:    factory.observer, failureTTL: min(factory.config.RequestTimeout, time.Minute), successTTL: time.Hour,
		}
	}
	clients.CostExplorer = costexplorer.NewFromConfig(sdkConfig, func(options *costexplorer.Options) {
		options.AppID = "aws-cost-exporter"
		options.Retryer = clients.Retryer(awscommon.OperationGetCostAndUsage)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.CostExplorer); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	clients.Athena = athena.NewFromConfig(sdkConfig, func(options *athena.Options) {
		options.Retryer = clients.Retryer(awscommon.OperationStartQueryExecution)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.Athena); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	clients.Organizations = organizations.NewFromConfig(sdkConfig, func(options *organizations.Options) {
		options.AppID = "aws-cost-exporter"
		options.Retryer = clients.Retryer(awscommon.OperationListAccounts)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.Organizations); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	clients.Budgets = budgets.NewFromConfig(sdkConfig, func(options *budgets.Options) {
		options.AppID = "aws-cost-exporter"
		options.Retryer = clients.Retryer(awscommon.OperationDescribeBudgets)
		if endpoint := strings.TrimSpace(factory.config.Endpoints.Budgets); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	return clients, nil
}

type identityAPI interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

type targetVerifier struct {
	client      identityAPI
	target      identity.TargetID
	accountID   string
	credentials aws.CredentialsProvider
	observer    awscommon.Observer
	failureTTL  time.Duration
	successTTL  time.Duration

	mu            sync.Mutex
	verified      bool
	verifiedUntil time.Time
	verifiedFor   [sha256.Size]byte
	inFlight      chan struct{}
	lastErr       error
	retryAfter    time.Time
	failedFor     [sha256.Size]byte
}

func (verifier *targetVerifier) Verify(ctx context.Context) error {
	if verifier == nil || verifier.client == nil || verifier.credentials == nil || verifier.target == "" || verifier.accountID == "" {
		return errors.New("invalid AWS target verifier")
	}
	for {
		fingerprint, err := verifier.credentialFingerprint(ctx)
		if err != nil {
			return err
		}
		verifier.mu.Lock()
		now := time.Now()
		if verifier.verified && verifier.verifiedFor == fingerprint &&
			(verifier.successTTL <= 0 || now.Before(verifier.verifiedUntil)) {
			verifier.mu.Unlock()
			return nil
		}
		verifier.verified = false
		if verifier.inFlight != nil {
			wait := verifier.inFlight
			verifier.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return awscommon.ClassifyError(ctx.Err())
			}
		}
		if verifier.lastErr != nil && verifier.failedFor == fingerprint && time.Now().Before(verifier.retryAfter) {
			err := verifier.lastErr
			verifier.mu.Unlock()
			return err
		}
		wait := make(chan struct{})
		verifier.inFlight = wait
		verifier.mu.Unlock()

		err = verifier.verify(ctx, fingerprint)
		verifier.mu.Lock()
		if err == nil {
			verifier.verified = true
			verifier.verifiedUntil = time.Now().Add(verifier.successTTL)
			verifier.verifiedFor = fingerprint
			verifier.lastErr = nil
		} else {
			verifier.lastErr = err
			verifier.retryAfter = time.Now().Add(verifier.failureTTL)
			verifier.failedFor = fingerprint
		}
		verifier.inFlight = nil
		close(wait)
		verifier.mu.Unlock()
		return err
	}
}

func (verifier *targetVerifier) verify(ctx context.Context, expected [sha256.Size]byte) error {
	started := time.Now()
	output, err := verifier.client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	awscommon.ObserveCall(verifier.observer, verifier.target, awscommon.OperationGetCallerIdentity, started, err)
	if err != nil {
		return awscommon.ClassifyError(err)
	}
	if output == nil || output.Account == nil || *output.Account != verifier.accountID {
		return identityMismatchError{}
	}
	current, err := verifier.credentialFingerprint(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return credentialsChangedError{}
	}
	return nil
}

func (verifier *targetVerifier) credentialFingerprint(ctx context.Context) ([sha256.Size]byte, error) {
	resolved, err := verifier.credentials.Retrieve(ctx)
	if err != nil {
		return [sha256.Size]byte{}, awscommon.ClassifyError(err)
	}
	if resolved.AccessKeyID == "" {
		return [sha256.Size]byte{}, awscommon.ClassifyError(errors.New("AWS credentials are unavailable"))
	}
	value := resolved.AccessKeyID + "\x00" + resolved.SecretAccessKey + "\x00" + resolved.SessionToken + "\x00" + resolved.Source
	if resolved.CanExpire {
		value += "\x00" + resolved.Expires.UTC().Format(time.RFC3339Nano)
	}
	return sha256.Sum256([]byte(value)), nil
}

type identityMismatchError struct{}

func (identityMismatchError) Error() string {
	return "AWS target identity does not match configured account"
}
func (identityMismatchError) SafeKind() string { return string(awscommon.ErrorValidation) }
func (identityMismatchError) Retryable() bool  { return false }

type credentialsChangedError struct{}

func (credentialsChangedError) Error() string {
	return "AWS credentials changed during target verification"
}
func (credentialsChangedError) SafeKind() string { return string(awscommon.ErrorTransient) }
func (credentialsChangedError) Retryable() bool  { return true }

type unavailableVerifier struct{ cause error }

func (verifier unavailableVerifier) Verify(context.Context) error { return verifier }
func (verifier unavailableVerifier) Error() string                { return "AWS credential source unavailable" }
func (verifier unavailableVerifier) Unwrap() error                { return verifier.cause }
func (unavailableVerifier) SafeKind() string                      { return string(awscommon.ErrorValidation) }
func (unavailableVerifier) Retryable() bool                       { return false }

func sessionName(target identity.TargetID, configured string) string {
	if configured != "" {
		return configured
	}
	value := "aws-cost-exporter-" + string(target)
	if len(value) > 64 {
		return value[:64]
	}
	return value
}

type observedSTS struct {
	client   *sts.Client
	target   identity.TargetID
	observer awscommon.Observer
}

func (client observedSTS) AssumeRole(ctx context.Context, input *sts.AssumeRoleInput, options ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	started := time.Now()
	output, err := client.client.AssumeRole(ctx, input, options...)
	awscommon.ObserveCall(client.observer, client.target, awscommon.OperationAssumeRole, started, err)
	return output, err
}
