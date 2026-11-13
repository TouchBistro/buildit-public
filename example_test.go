package buildit_test

import (
	"context"
	"log"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit"
	"github.com/TouchBistro/buildit/resource"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// A program provisions a log group and a certificate in the account it runs in, and a
// second log group in another account reached by assuming a role. The providers are
// built with the awesome providers package, which registers them; buildit finds them
// there by name.
func ExampleNewConfig() {
	ctx := context.Background()

	// `default` is the program's own credentials. Register it explicitly to pick the
	// region and validate it up front; NewConfig builds one from the AWS default
	// credentials chain if none is registered.
	base, err := providers.NewDefaultCredsProvider(ctx, providers.DefaultCredsProviderName,
		providers.WithRegion("us-east-1"), providers.ValidateProvider())
	if err != nil {
		log.Fatal(err)
	}
	other, err := providers.NewAssumeRoleCredsProvider(ctx, "example-other",
		providers.WithBaseCredsProvider(base),
		providers.WithAccountId("123456789012"),
		providers.WithRoleName("example-role"),
		providers.WithRegion("us-east-1"))
	if err != nil {
		log.Fatal(err)
	}

	var rs buildit.ResourceSet
	logs := resource.CWLogGroup{Retention: aws.Int32(30)}
	if err := rs.AddCWLogGroup("example-logs", logs); err != nil { // provisioned by `main`, here `default`
		log.Fatal(err)
	}
	if err := rs.AddCWLogGroupWithProvider(other, "example-logs", logs); err != nil {
		log.Fatal(err)
	}
	cert := resource.ACMCertificate{
		DomainName: "api.example.com",
		DependsOn:  []resource.Key{resource.NewKey("example-logs")},
	}
	if err := rs.AddCertificate("example-cert", cert); err != nil {
		log.Fatal(err)
	}

	cfg, err := buildit.NewConfig(ctx, rs, buildit.WithGlobalTag("team", "example-team"))
	if err != nil {
		log.Fatal(err)
	}

	ctx = buildit.ContextWithRetryPolicy(ctx, 3, 20)
	if err := cfg.Graph().Plan(ctx); err != nil {
		log.Fatal(err)
	}
}
