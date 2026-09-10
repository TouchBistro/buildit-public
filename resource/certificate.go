package resource

import (
	"context"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

const (
	retriesRemaining = 40
	waitInSeconds    = 15
)

// ACMCertificate represents a CSR for ACM Certificates
//
// Name (the config key) is the resource's identity and its buildit:resource-id tag;
// DomainName is the certificate's CN and defaults to Name. Keeping them separate (same
// pattern as route53-record's recordName) lets two certificates share a CN in one config
// scope under distinct names, with the resource-id tag telling their lifecycles apart.
type ACMCertificate struct {
	BaseResource     `yaml:",inline"`
	Name             string            `yaml:"-"`                       // resource name (config key); the Identifier and buildit:resource-id
	DomainName       string            `yaml:"domainName"`              // the main domain (CN) for the CSR; defaults to Name
	SAN              []string          `yaml:"san"`                     // a list of subject alternative names for the CSR
	ValidationDomain string            `yaml:"dnsValidationDomainName"` // domain to use for validationin [provider/]domain-name format
	Tags             map[string]string `yaml:"tags"`                    // tags to be added
	GlobalTags       map[string]string `yaml:"-"`
	DependsOn        []Key             `yaml:"-"`
}

// Key returns the unique key for the resource for this buildit context
func (c ACMCertificate) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns the resource name, which defaults to the certificate's domain
func (c ACMCertificate) Identifier() string {
	return c.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (c *ACMCertificate) Normalize(ctx context.Context) {

	// the certificate CN defaults to the resource name
	if c.DomainName == "" {
		c.DomainName = c.Name
	}

	// add a period if not supplied
	if !strings.HasSuffix(c.ValidationDomain, ".") {
		c.ValidationDomain += "."
	}

	// merge globalTags to certificate tags
	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}

	ResourceTags(c.Tags).Merge(c.GlobalTags)
}

// Validate checks that the input provided is correct
func (c ACMCertificate) Validate(ctx context.Context) error {
	return nil
}

// Apply request a new certificate, and performs DNS domain validation
func (c ACMCertificate) Apply(ctx context.Context) error {
	log.Debugf("creating certificate %v", c.Identifier())

	diffs, err := c.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", c.Identifier()).Info("certificate already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = c.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update certificate %v", c.Identifier())
		}
		return nil
	}

	return c.apply(ctx)
}

// Destroy removes the certificate
func (c ACMCertificate) Destroy(ctx context.Context) error {
	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return errors.Wrap(err, "error listing certificate")
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Domain Name": c.Identifier(),
		}).Info("certificate does not exist, nothing to destroy, skippping ")
		return nil
	}

	// first remove DNS validation record(s) for this certificate
	err = c.manageDNSValidation(ctx, existing.DomainValidationOptions, true)
	if err != nil {
		return errors.Wrapf(err, "error deleting validation records %v", c.DomainName)
	}

	// now delete the certificate
	_, err = client.ACM(ctx, c.Context.ProviderName).DeleteCertificate(ctx, &acm.DeleteCertificateInput{
		CertificateArn: existing.CertificateArn,
	})

	// TODO: @esiddiqui check if we want to wait for deletion ...

	if err != nil {
		return errors.Wrapf(err, "error deleting certificate %v", c.DomainName)
	}

	log.WithFields(log.Fields{
		"Domain Name": *existing.DomainName,
	}).Infof("%s", color.Red("crtificate deleted"))

	return nil
}

type ACMCertificateDiff struct {
	BaseResourceDiff

	domainDiff     bool
	sansDiff       bool
	beingValidated bool
	tagsDiff       bool
	tagDiff        util.TagDiffResult
}

// Compare fetches the existing certificate, and if it exists, checks if this
// resource is equal to the corresponding aws certficiate
func (c ACMCertificate) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", c.Identifier())
	}

	diffs := &ACMCertificateDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "certificate does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	diff := false

	// domain
	if c.DomainName != *existing.DomainName {
		diff = true
		diffs.domainDiff = true
		diffs.Messages = append(diffs.Messages, "certificate domain name is different")
	}

	// san
	sans := make([]string, len(c.SAN))
	copy(sans, c.SAN)
	sans = append(sans, c.DomainName)
	if !util.SliceElementsEqual(sans, existing.SubjectAlternativeNames) {
		diff = true
		diffs.sansDiff = true
		diffs.Messages = append(diffs.Messages, "certificate subject alternative name (san) are different")
	}

	// validation status
	if existing.Status == acmtypes.CertificateStatusPendingValidation {
		diff = true
		diffs.beingValidated = true
		diffs.Messages = append(diffs.Messages, "certificate current state is validating")
	}

	// tags
	awsTags, err := awsw.NewACM(ctx, c.Context.ProviderName).GetResourceTags(ctx, *existing.CertificateArn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, c.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !diff {
		return nil, nil
	}

	return diffs, nil
}

// apply provisions are new acm certificate
func (c ACMCertificate) apply(ctx context.Context) error {
	acmClient := client.ACM(ctx, c.Context.ProviderName)

	//tags
	var tags []acmtypes.Tag
	for k, v := range c.Tags {
		tags = append(tags, acmtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	certResponse, err := acmClient.RequestCertificate(ctx, &acm.RequestCertificateInput{
		DomainName:              aws.String(c.DomainName),
		SubjectAlternativeNames: c.SAN,
		ValidationMethod:        acmtypes.ValidationMethodDns,
		Tags:                    tags,
	})

	if err != nil {
		return errors.Wrapf(err, "error requesting certificate for %v", c.DomainName)
	}

	certificateArn := certResponse.CertificateArn
	err = c.validateCertificate(ctx, certificateArn)
	if err != nil {
		return errors.Wrapf(err, "failed to confirm certificate validation within expected time for %v", c.DomainName)
	}

	log.WithFields(log.Fields{
		"Domain Name": c.DomainName,
	}).Infof("%s", color.Green("acm certificate issued"))

	return nil
}

// applyDiffs applies changes to the certificate
func (c ACMCertificate) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("no updates required for certificate")
		return nil
	}

	certDiffs, ok := diffs.(*ACMCertificateDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing cert
	existing, ok := certDiffs.Resource.(*acmtypes.CertificateDetail)
	if !ok {
		return errors.Errorf("invalid existing certificate supplied")
	}

	// domain
	if certDiffs.domainDiff {
		return errors.Errorf("certificate domain cannot be updated")
	}

	// san
	if certDiffs.sansDiff {
		return errors.Errorf("certificate subject alternative names (san) cannot be updated")
	}

	// validation
	if certDiffs.beingValidated {
		err := c.validateCertificate(ctx, existing.CertificateArn)
		if err != nil {
			return errors.Wrapf(err, "failed to confirm certificate validation within expected time for %v", c.DomainName)
		}
	}

	// tags
	var err error
	if certDiffs.tagsDiff {
		upserts := certDiffs.tagDiff.Upserts()

		if len(upserts) > 0 {
			err = awsw.NewACM(ctx, c.Context.ProviderName).AddResourceTags(ctx, *existing.CertificateArn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating certificate tags for %v", c.Identifier())
			}
		}

		if len(certDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewACM(ctx, c.Context.ProviderName).DeleteResourceTags(ctx, *existing.CertificateArn, certDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting certificate tags for %v", c.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name":     c.Identifier(),
		"Group ID": *existing.CertificateArn,
	}).Info(color.Yellow("certificate updated"))

	return nil
}

// fetchExisting returns the existing certificate details if found. The lookup goes
// through the shared awsw lifecycle resolver (DEVOPS-8880): candidates are matched by
// domain, and the buildit:resource-id tag — this resource's Name — decides which CN twin
// is this resource's own, so compare and destroy operate on the certificate buildit
// manages rather than an arbitrary twin.
func (c ACMCertificate) fetchExisting(ctx context.Context) (*acmtypes.CertificateDetail, error) {

	arn, err := awsw.NewACM(ctx, c.Context.ProviderName).FindCertificateForResource(ctx, c.DomainName, c.Identifier())
	if err != nil {
		return nil, errors.Wrapf(err, "error resolving acm certificate %v", c.Identifier())
	}
	if arn == nil {
		return nil, nil
	}

	descResp, err := client.ACM(ctx, c.Context.ProviderName).DescribeCertificate(ctx, &acm.DescribeCertificateInput{
		CertificateArn: arn,
	})
	if err != nil {
		// When a certificate is being deleted, it's possible that between the lookup
		// and DescribeCertificate the certificate has been removed. That is not an
		// error — the certificate simply doesn't exist anymore; so nil, nil.
		var rnfe *acmtypes.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error describing certificate %v", c.DomainName)
	}
	return descResp.Certificate, nil
}

// validateCertificate performs certificte validation
func (c ACMCertificate) validateCertificate(ctx context.Context, certificateArn *string) error {

	domainValidationOptions, err := waitUntilDomainValidationDetailsAvailable(ctx, c.Context.ProviderName, certificateArn)

	if err != nil {
		return errors.Wrap(err, "error fetching domain validation details")
	}

	err = c.manageDNSValidation(ctx, domainValidationOptions, false)
	if err != nil {
		return errors.Wrapf(err, "error creating domain validation DNS record for %v", c.DomainName)
	}

	//wait until the DNS vaidation succeeds...
	err = waitUntilCertificateValidated(ctx, c.Context.ProviderName, certificateArn)

	if err != nil {
		return errors.Wrapf(err, "failed to confirm certificate validation within expected time for %v", c.DomainName)
	}

	return nil

}

// waitUntilDomainValidationDetailsAvailable waits untils the certificate's domain validation options are available
// and returns them. If the certificate state at any time during the tests is not PENDING_VALIDATION, it will return and error.
// currently this function will wait 15s between each check & perform a maximum of 40 checks, so the maximum wait time
// before this function exits with a failure is 600s or 10m
func waitUntilDomainValidationDetailsAvailable(ctx context.Context, providerName string, certificateArn *string) ([]acmtypes.DomainValidation, error) {

	acmClient := client.ACM(ctx, providerName)
	done := false
	retries := retriesRemaining

	for !done {
		log.Debugf("waiting %v seconds to retrieve domain validation instructions", waitInSeconds)
		time.Sleep(time.Duration(waitInSeconds) * time.Second)

		descResp, err := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
			CertificateArn: certificateArn,
		})

		if err != nil {
			return nil, errors.Wrap(err, "error ferching certificate details")
		}

		if descResp.Certificate.Status != acmtypes.CertificateStatusPendingValidation {
			return nil, errors.Errorf("this certificate %v is in %v state, expected PENDING_VALIDATION",
				*descResp.Certificate.DomainName, descResp.Certificate.Status)
		}

		opts := descResp.Certificate.DomainValidationOptions
		done = true //assume done
		for _, opt := range opts {
			if opt.ResourceRecord == nil {
				done = false
				break
			}
		}

		if done {
			return descResp.Certificate.DomainValidationOptions, nil
		}

		retries--
		done = retries == 0
	}

	return nil, errors.New("could not retrieve domain validation information within specified time ")
}

// waitUntilCertificateValidated waits untils the certificate validation is complete and the certificate is
// in issued state. If the certificate state at any time during the tests is not PENDING_VALIDATION | ISSUED,
// it will return and error. currently this function will wait 15s between each check & perform a maximum of
// 40 checks, so the maximum wait time before this function exits with a failure is 600s or 10m
func waitUntilCertificateValidated(ctx context.Context, providerName string, certificateArn *string) error {

	acmClient := client.ACM(ctx, providerName)
	done := false
	retries := retriesRemaining

	for !done {
		log.Debugf("waiting %v seconds to check certificate status", waitInSeconds)
		time.Sleep(time.Duration(waitInSeconds) * time.Second)

		descResp, err := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
			CertificateArn: certificateArn,
		})

		if err != nil {
			return errors.Wrap(err, "error fetching certificate details")
		}

		if descResp.Certificate.Status == acmtypes.CertificateStatusIssued {
			return nil
		}

		if descResp.Certificate.Status != acmtypes.CertificateStatusPendingValidation {
			return errors.Errorf("this certificate %v is in %v state, expected PENDING_VALIDATION",
				*descResp.Certificate.DomainName, descResp.Certificate.Status)
		}

		retries--
		done = retries == 0
	}

	return errors.New("couldln't validate certificate within specified time")
}

// manageDNSValidation uses the validation information provided to add/upsert or remove corresponding DNS
// validation entries for the certificate domain name and all subject alternative names (SAN)
func (c ACMCertificate) manageDNSValidation(ctx context.Context, validations []acmtypes.DomainValidation, clear bool) error {

	//check for duplicate validation records
	validationDupes := make(map[string]string)

	//build a change record for each validation; ignoring the duplicate ones
	for _, val := range validations {
		//de-dupe the validation records, since sometimes the SAN with a wildcard
		//results in an identical DNS validation record
		if _, ok := validationDupes[*val.ResourceRecord.Name]; ok {
			continue
		}

		delim := "::"
		// check if :: is not used as delimiter, then use legacy /
		if !strings.Contains(c.ValidationDomain, "::") {
			delim = "/"
		}
		providerName, validationDomain := NewKeyWithDelim(delim, c.ValidationDomain).Split()

		recordFqdn := val.ResourceRecord.Name
		destination := *val.ResourceRecord.Value

		// string the namespace/domain fromt he validation record name
		recordName := *recordFqdn
		if strings.HasSuffix(*recordFqdn, validationDomain) {
			recordName = recordName[:len(recordName)-len(validationDomain)-1] // strip out the validation domain part to form the record name part only
		}
		recordName = providerName + "/" + recordName // append provider
		record := Route53Record{
			BaseResource: BaseResource{
				Context: Context{
					ProviderName: providerName,
				},
			},
			Name:         recordName,
			HostedZone:   validationDomain,
			TTL:          aws.Int64(10800),
			Type:         aws.String(RecordTypeCNAME),
			Destinations: []string{destination},
		}
		record.Normalize(ctx)

		if !clear {
			err := record.apply(ctx)
			if err != nil {
				return errors.Wrapf(err, "failed to add validation dns record %v", recordFqdn)
			}
		} else {
			err := record.Destroy(ctx)
			if err != nil {
				return errors.Wrapf(err, "failed to remove validation dns record %v", recordFqdn)
			}
		}
		validationDupes[*val.ResourceRecord.Name] = *val.ResourceRecord.Value //add to dupes
	}

	return nil
}
