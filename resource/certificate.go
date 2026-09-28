package resource

import (
	"context"
	"fmt"
	"maps"
	"slices"
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
	ValidationDomain string            `yaml:"dnsValidationDomainName"` // optional hosted zone for every validation CNAME, [provider/]zone or provider::zone; empty = discover per record
	ValidationZones  map[string]string `yaml:"dnsValidationZones"`      // optional hosted zone per domain (CN and each SAN), same value format; every domain must be listed
	Tags             map[string]string `yaml:"tags"`                    // tags to be added
	GlobalTags       map[string]string `yaml:"-"`
	DependsOn        []Key             `yaml:"-"`

	validationZones map[string]string // explicit placement per normalized domain, folded from the two fields above by Normalize; nil = discover
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

	// add a period if not supplied; empty means the hosted zone is discovered per validation record
	if c.ValidationDomain != "" && !strings.HasSuffix(c.ValidationDomain, ".") {
		c.ValidationDomain += "."
	}
	c.validationZones = c.resolveValidationZones()

	// merge globalTags to certificate tags
	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}

	ResourceTags(c.Tags).Merge(c.GlobalTags)
}

// resolveValidationZones folds the two explicit placement fields into one lookup keyed by
// normalized domain, so Compare, Apply and Destroy consult a single source. dnsValidationZones
// wins; a lone dnsValidationDomainName still means "every domain in that one zone"; neither
// leaves the map nil and the zone to discovery. Discovery itself stays out of Normalize: it
// reaches Route53 and must fail the plan with a scoped error, not every command with a panic.
func (c ACMCertificate) resolveValidationZones() map[string]string {
	switch {
	case len(c.ValidationZones) > 0:
		zones := make(map[string]string, len(c.ValidationZones))
		for domain, zone := range c.ValidationZones {
			if zone != "" && !strings.HasSuffix(zone, ".") {
				zone += "."
			}
			zones[awsw.NormalizeDNSName(domain)] = zone
		}
		return zones
	case c.ValidationDomain != "":
		zones := make(map[string]string, len(c.SAN)+1)
		for _, domain := range c.certificateDomains() {
			zones[awsw.NormalizeDNSName(domain)] = c.ValidationDomain
		}
		return zones
	default:
		return nil
	}
}

// certificateDomains is the CN followed by every SAN, as written in the config.
func (c ACMCertificate) certificateDomains() []string {
	return append([]string{c.DomainName}, c.SAN...)
}

// Validate checks that the input provided is correct. dnsValidationZones is all-or-nothing:
// when set it must name a zone for the CN and every SAN (a mapped subset would silently mix
// explicit placement with discovery), and it cannot be combined with dnsValidationDomainName.
func (c ACMCertificate) Validate(ctx context.Context) error {
	var msgs []string

	if len(c.ValidationZones) > 0 {
		if c.ValidationDomain != "" {
			msgs = append(msgs, "set either dnsValidationZones or dnsValidationDomainName, not both")
		}

		domains := make(map[string]string, len(c.SAN)+1) // normalized → as written
		for _, d := range c.certificateDomains() {
			domains[awsw.NormalizeDNSName(d)] = d
		}
		covered := make(map[string]struct{}, len(c.ValidationZones))
		for _, key := range slices.Sorted(maps.Keys(c.ValidationZones)) {
			normalized := awsw.NormalizeDNSName(key)
			if _, ok := domains[normalized]; !ok {
				msgs = append(msgs, fmt.Sprintf("dnsValidationZones key %q is not the certificate domain or a san", key))
			}
			if strings.TrimSpace(c.ValidationZones[key]) == "" {
				msgs = append(msgs, fmt.Sprintf("dnsValidationZones entry %q has no hosted zone", key))
			}
			covered[normalized] = struct{}{}
		}
		for _, normalized := range slices.Sorted(maps.Keys(domains)) {
			if _, ok := covered[normalized]; !ok {
				msgs = append(msgs, fmt.Sprintf("dnsValidationZones has no entry for %q; list every domain on the certificate, or omit the field to discover zones", domains[normalized]))
			}
		}
	}

	if len(msgs) == 0 {
		return nil
	}
	return &ValidationError{
		ResourceType:       "certificate",
		ResourceIdentifier: c.Identifier(),
		Messages:           msgs,
	}
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

	// A certificate that still needs its validation records written must be able to place
	// them: fail the plan here, before RequestCertificate, when no zone can be discovered.
	if existing == nil || existing.Status == acmtypes.CertificateStatusPendingValidation {
		if err := c.checkValidationZones(ctx); err != nil {
			return nil, err
		}
	}

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
// domain, and the buildit:resource-id tag — this resource's Name — decides which of the
// same-CN certificates is this resource's own, so compare and destroy operate on the
// certificate buildit manages rather than an arbitrary one.
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
// validation entries for the certificate domain name and all subject alternative names (SAN). Each record
// is placed in the zone resolveValidationTarget picks for it, so the create and destroy paths always agree
// on where a record lives.
func (c ACMCertificate) manageDNSValidation(ctx context.Context, validations []acmtypes.DomainValidation, clear bool) error {

	//check for duplicate validation records
	validationDupes := make(map[string]string)

	//build a change record for each validation; ignoring the duplicate ones
	for _, val := range validations {
		// ACM has not produced a record yet (a destroy racing a fresh request); nothing to place or remove
		if val.ResourceRecord == nil || val.ResourceRecord.Name == nil || val.ResourceRecord.Value == nil {
			log.WithField("Domain", aws.ToString(val.DomainName)).Debug("validation option has no resource record yet, skipping")
			continue
		}
		//de-dupe the validation records, since sometimes the SAN with a wildcard
		//results in an identical DNS validation record
		if _, ok := validationDupes[*val.ResourceRecord.Name]; ok {
			continue
		}

		recordFqdn := *val.ResourceRecord.Name
		destination := *val.ResourceRecord.Value

		target, err := c.resolveValidationTarget(ctx, aws.ToString(val.DomainName), recordFqdn)
		if err != nil {
			return err
		}

		record := Route53Record{
			BaseResource: BaseResource{
				Context: Context{
					ProviderName: target.provider,
				},
			},
			Name:         target.provider + "/" + validationRecordName(recordFqdn, target.zone),
			HostedZone:   target.zone,
			TTL:          aws.Int64(10800),
			Type:         aws.String(RecordTypeCNAME),
			Destinations: []string{destination},
			hostedZoneId: target.zoneID, // set when discovered; Normalize looks it up by name otherwise
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

// validationTarget is where one validation CNAME lives: the provider whose Route53 holds the
// zone, the zone name (trailing dot), and the zone id when discovery already resolved it.
type validationTarget struct {
	provider string
	zone     string
	zoneID   *string
}

// resolveValidationTarget decides the hosted zone for the validation record of one certificate
// domain (the CN or a SAN, as ACM reports it). An explicit placement — dnsValidationZones for
// that domain, or a lone dnsValidationDomainName for all of them — is honoured exactly as
// written and never reaches AWS (DEVOPS-8968 keeps existing configs byte-for-byte compatible).
// Otherwise the zone is discovered in the certificate's own provider as the longest public
// suffix of the record — see awsw.Route53.FindHostedZoneForRecord.
func (c ACMCertificate) resolveValidationTarget(ctx context.Context, domain, recordFqdn string) (validationTarget, error) {
	if explicit, ok := c.validationZones[awsw.NormalizeDNSName(domain)]; ok {
		provider, zone := explicitValidationTarget(explicit)
		return validationTarget{provider: provider, zone: zone}, nil
	}
	// a domain ACM spells differently from the config still lands in the legacy single zone
	if c.ValidationDomain != "" {
		provider, zone := explicitValidationTarget(c.ValidationDomain)
		return validationTarget{provider: provider, zone: zone}, nil
	}

	hz, err := awsw.NewRoute53(ctx, c.Context.ProviderName).FindHostedZoneForRecord(ctx, recordFqdn)
	if err != nil {
		return validationTarget{}, errors.Wrapf(err, "cannot place validation dns record for certificate %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name":        c.Identifier(),
		"Record":      recordFqdn,
		"Hosted Zone": aws.ToString(hz.Name),
	}).Debug("discovered hosted zone for validation record")

	return validationTarget{
		provider: c.Context.ProviderName,
		zone:     aws.ToString(hz.Name),
		zoneID:   hz.Id,
	}, nil
}

// explicitValidationTarget splits a dnsValidationDomainName or dnsValidationZones value into
// provider and zone. Both the legacy "provider/zone" and the "provider::zone" forms are
// accepted; a bare zone means the main provider. The zone keeps the trailing dot Normalize added.
func explicitValidationTarget(validationDomain string) (provider, zone string) {
	delim := "::"
	// check if :: is not used as delimiter, then use legacy /
	if !strings.Contains(validationDomain, "::") {
		delim = "/"
	}
	return NewKeyWithDelim(delim, validationDomain).Split()
}

// validationRecordName strips the zone from a validation record FQDN, leaving the relative
// record name Route53Record expects ("_abc.api" for "_abc.api.example.com." in "example.com.").
// A record that is not under the zone (an explicit dnsValidationDomainName that does not own
// it) is passed through untouched, exactly as before.
func validationRecordName(recordFqdn, zone string) string {
	if recordFqdn != zone && strings.HasSuffix(recordFqdn, "."+zone) {
		return strings.TrimSuffix(recordFqdn, "."+zone)
	}
	return recordFqdn
}

// checkValidationZones is the plan-time half of zone discovery: every domain left to discovery
// must resolve to a public hosted zone in the resource's provider, or the plan fails with the
// discovery error before ACM is touched. A validation record "_abc.<domain>." is one leaf label
// below its domain, so the zone that owns the domain is the zone that owns the record. Domains
// with an explicit placement are skipped: the operator has named the zone, and the record is
// written there whatever it is.
func (c ACMCertificate) checkValidationZones(ctx context.Context) error {
	domains := validationLookupDomains(c.discoveredDomains())
	if len(domains) == 0 {
		return nil
	}

	r53 := awsw.NewRoute53(ctx, c.Context.ProviderName)
	for _, domain := range domains {
		if _, err := r53.FindHostedZoneForRecord(ctx, domain); err != nil {
			return errors.Wrapf(err, "cannot place validation dns record for certificate %v", c.Identifier())
		}
	}
	return nil
}

// discoveredDomains lists the CN and SANs whose hosted zone is left to discovery: those without
// an explicit placement. A lone dnsValidationDomainName covers every domain, so none remain.
func (c ACMCertificate) discoveredDomains() []string {
	if c.ValidationDomain != "" {
		return nil
	}
	var domains []string
	for _, d := range c.certificateDomains() {
		if _, ok := c.validationZones[awsw.NormalizeDNSName(d)]; !ok {
			domains = append(domains, d)
		}
	}
	return domains
}

// validationLookupDomains returns the distinct domains whose hosted zone must exist for the
// certificate to validate, with a leading wildcard label removed (the validation record for
// "*.example.com" is "_abc.example.com.").
func validationLookupDomains(domains []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, d := range domains {
		d = awsw.NormalizeDNSName(strings.TrimPrefix(d, "*."))
		if _, dup := seen[d]; d == "." || dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}
