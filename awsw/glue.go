package awsw

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
)

type Glue struct {
	*glue.Client
}

func NewGlue(ctx context.Context, providerName string) Glue {
	return Glue{client.Glue(ctx, providerName)}
}

// CatalogArnForIdentifier resolves a Glue Catalog ARN from an identifier (ARN, catalog ID, or catalog name).
func (g Glue) CatalogArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	glueService := g
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		glueService = NewGlue(ctx, provider)
	}

	if strings.EqualFold(resource, "catalog") {
		stsProvider := provider
		if stsProvider == "" {
			stsProvider = client.MainProvider
		}
		stsClient := NewSTS(ctx, stsProvider)
		accountID, err := stsClient.GetAccountID(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get account ID for glue catalog lookup: %w", err)
		}

		region := glueService.Options().Region
		arn := fmt.Sprintf("arn:aws:glue:%s:%s:catalog", region, accountID)
		return &arn, nil
	}

	// Tier 1: direct lookup by catalog ID/name.
	out, err := glueService.GetCatalog(ctx, &glue.GetCatalogInput{
		CatalogId: aws.String(resource),
	})
	if err == nil && out.Catalog != nil && out.Catalog.ResourceArn != nil && aws.ToString(out.Catalog.ResourceArn) != "" {
		return out.Catalog.ResourceArn, nil
	}

	// Tier 2: list and match by catalog name.
	var nextToken *string
	var matchArn *string
	matchCount := 0
	for {
		listOut, listErr := glueService.GetCatalogs(ctx, &glue.GetCatalogsInput{
			IncludeRoot: aws.Bool(true),
			NextToken:   nextToken,
		})
		if listErr != nil {
			if err != nil {
				return nil, fmt.Errorf("failed to get glue catalog %q (%v) and list catalogs: %w", resource, err, listErr)
			}
			return nil, fmt.Errorf("failed to list glue catalogs: %w", listErr)
		}

		for _, catalog := range listOut.CatalogList {
			if !strings.EqualFold(aws.ToString(catalog.Name), resource) {
				continue
			}
			if catalog.ResourceArn == nil || aws.ToString(catalog.ResourceArn) == "" {
				continue
			}
			matchCount++
			if matchCount == 1 {
				matchArn = catalog.ResourceArn
			}
		}

		if listOut.NextToken == nil {
			break
		}
		nextToken = listOut.NextToken
	}

	if matchCount > 1 {
		return nil, fmt.Errorf("ambiguous glue catalog identifier %q: found %d matches", resource, matchCount)
	}
	if matchCount == 1 {
		return matchArn, nil
	}

	return nil, fmt.Errorf("glue catalog %q not found", resource)
}
