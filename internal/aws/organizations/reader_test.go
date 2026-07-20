package organizations

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsorganizations "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"

	awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"
)

type fakeOrganizations struct {
	listCalls int
	duplicate bool
}

func (value *fakeOrganizations) DescribeOrganization(context.Context, *awsorganizations.DescribeOrganizationInput, ...func(*awsorganizations.Options)) (*awsorganizations.DescribeOrganizationOutput, error) {
	return &awsorganizations.DescribeOrganizationOutput{Organization: &orgtypes.Organization{Id: aws.String("o-example")}}, nil
}
func (value *fakeOrganizations) ListAccounts(_ context.Context, input *awsorganizations.ListAccountsInput, _ ...func(*awsorganizations.Options)) (*awsorganizations.ListAccountsOutput, error) {
	value.listCalls++
	if input.NextToken == nil {
		return &awsorganizations.ListAccountsOutput{Accounts: []orgtypes.Account{{Id: aws.String("111111111111"), Name: aws.String("production"), Email: aws.String("private@example.com"), State: orgtypes.AccountStateActive}}, NextToken: aws.String("next")}, nil
	}
	id := "222222222222"
	if value.duplicate {
		id = "111111111111"
	}
	return &awsorganizations.ListAccountsOutput{Accounts: []orgtypes.Account{{Id: aws.String(id), Name: aws.String("suspended"), State: orgtypes.AccountStateSuspended}}}, nil
}

func TestReaderPaginatesMapsStateAndDropsEmail(t *testing.T) {
	api := &fakeOrganizations{}
	reader, err := New("payer", api, 3, Policy{SeriesLimit: 2}, nil, func(string) aws.Retryer { return aws.NopRetryer{} })
	if err != nil {
		t.Fatal(err)
	}
	values, err := reader.ReadAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if api.listCalls != 2 || len(values) != 2 || values[0].Name != "production" || values[0].Status != "ACTIVE" || values[1].Status != "SUSPENDED" {
		t.Fatalf("accounts=%#v calls=%d", values, api.listCalls)
	}
}

func TestReaderRejectsDuplicateAccountAndInvalidConfig(t *testing.T) {
	reader, _ := New("payer", &fakeOrganizations{duplicate: true}, 3, Policy{SeriesLimit: 2}, awscommon.DiscardObserver{}, func(string) aws.Retryer { return aws.NopRetryer{} })
	if _, err := reader.ReadAccounts(context.Background()); err == nil {
		t.Fatal("accepted duplicate account")
	}
	if reader, err := New("", &fakeOrganizations{}, 1, Policy{SeriesLimit: 1}, nil, func(string) aws.Retryer { return aws.NopRetryer{} }); reader != nil || err == nil {
		t.Fatal("accepted empty target")
	}
}

func TestReaderFiltersAllowlistBeforePublicationAndBoundsObservedMode(t *testing.T) {
	reader, err := New("payer", &fakeOrganizations{}, 3, Policy{AccountIDs: []string{"222222222222"}, SeriesLimit: 1}, nil, func(string) aws.Retryer { return aws.NopRetryer{} })
	if err != nil {
		t.Fatal(err)
	}
	values, err := reader.ReadAccounts(context.Background())
	if err != nil || len(values) != 1 || values[0].AccountID != "222222222222" {
		t.Fatalf("allowlisted accounts=%#v err=%v", values, err)
	}
	bounded, err := New("payer", &fakeOrganizations{}, 3, Policy{SeriesLimit: 1}, nil, func(string) aws.Retryer { return aws.NopRetryer{} })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bounded.ReadAccounts(context.Background()); !errors.Is(err, ErrSeriesLimit) {
		t.Fatalf("ReadAccounts()=%v, want ErrSeriesLimit", err)
	}
}
