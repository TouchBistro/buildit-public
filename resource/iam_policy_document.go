package resource

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/TouchBistro/buildit/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// principalKeyCasing folds principal type keys to AWS's canonical casing.
var principalKeyCasing = map[string]string{
	"aws":           "AWS",
	"service":       "Service",
	"federated":     "Federated",
	"canonicaluser": "CanonicalUser",
}

// CanonicalPrincipalKey returns the AWS-canonical casing for a principal type key (AWS, Service,
// Federated, CanonicalUser); unknown keys are returned unchanged. AWS canonicalizes these keys
// when storing a policy, so desired-side keys must be folded the same way or a lowercase key in
// config (e.g. `service:`) registers as a perpetual diff against what AWS returns.
func CanonicalPrincipalKey(k string) string {
	if ck, ok := principalKeyCasing[strings.ToLower(k)]; ok {
		return ck
	}
	return k
}

// UnmarshalJSON implements custom JSON decoding for IAM policy documents.
// The IAM policy grammar allows Statement to be either a single statement
// object or an array of statements, so both forms are accepted.
func (d *IAMPolicyDocument) UnmarshalJSON(data []byte) error {
	var raw struct {
		Version   string          `json:"Version"`
		ID        string          `json:"Id"`
		Statement json.RawMessage `json:"Statement"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "failed to decode IAM policy document")
	}

	d.Version = raw.Version
	d.ID = raw.ID
	d.Statement = nil
	if len(raw.Statement) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw.Statement, &d.Statement); err == nil {
		return nil
	}

	var single IAMPolicyStatement
	if err := json.Unmarshal(raw.Statement, &single); err != nil {
		return errors.Wrap(err, "failed to decode IAM policy statement")
	}
	d.Statement = []IAMPolicyStatement{single}
	return nil
}

// equals compares this IAM policy document with the supplied other & returns
// Equal if the two are semantically equal. Documents are compared in a
// canonical form so that representation-only differences do not register as
// changes:
//
//   - a bare string equals the equivalent single-element array for union-typed
//     fields (Action, NotAction, Resource, NotResource, Principal.*,
//     NotPrincipal.* and condition values)
//   - ordering of values within those lists is insignificant
//   - ordering of statements is insignificant, with or without Sids: IAM
//     evaluates all statements together and an explicit deny always overrides
//     an allow, so statement order never changes the meaning of a policy
//
// Note "Principal": "*" and "Principal": {"AWS": "*"} are deliberately kept
// distinct as AWS does not treat them identically in all contexts.
func (d *IAMPolicyDocument) equals(other *IAMPolicyDocument) EqualsResult {
	switch {
	case d == nil && other == nil:
		return Equal
	case d == nil:
		return LeftZero
	case other == nil:
		return RightZero
	}

	dc, dErr := d.canonical()
	oc, oErr := other.canonical()
	if dErr != nil || oErr != nil {
		// Canonicalization should never fail for a valid policy document. Report
		// NotEqual rather than falling back to a comparison with different
		// semantics: the safe failure mode is a spurious update, never masked drift.
		log.WithFields(log.Fields{
			"leftError":  dErr,
			"rightError": oErr,
		}).Warn("failed to canonicalize IAM policy document, treating documents as different")
		return NotEqual
	}

	if dc == oc {
		return Equal
	}
	return NotEqual
}

// canonical returns a stable string representation of the policy document
// used for semantic comparison.
func (d *IAMPolicyDocument) canonical() (string, error) {
	stmts := make([]string, len(d.Statement))
	for i, s := range d.Statement {
		cs, err := s.canonical()
		if err != nil {
			return "", err
		}
		stmts[i] = cs
	}
	// Statement order is insignificant; sort so equal sets compare equal.
	slices.Sort(stmts)

	doc := map[string]any{"Statement": stmts}
	if d.Version != "" {
		doc["Version"] = d.Version
	}
	if d.ID != "" {
		doc["Id"] = d.ID
	}

	b, err := json.Marshal(doc)
	if err != nil {
		return "", errors.Wrap(err, "failed to encode canonical policy document")
	}
	return string(b), nil
}

// canonical returns a stable string representation of the policy statement.
// json.Marshal of a map produces keys in sorted order, which makes the
// resulting string deterministic.
func (s IAMPolicyStatement) canonical() (string, error) {
	stmt := map[string]any{}
	if s.Sid != "" {
		stmt["Sid"] = s.Sid
	}
	if s.Effect != "" {
		stmt["Effect"] = s.Effect
	}
	setIfNotNil(stmt, "Principal", normalizePrincipal(s.Principal))
	setIfNotNil(stmt, "NotPrincipal", normalizePrincipal(s.NotPrincipal))
	setIfNotNil(stmt, "Action", normalizeValueSet(s.Action))
	setIfNotNil(stmt, "NotAction", normalizeValueSet(s.NotAction))
	setIfNotNil(stmt, "Resource", normalizeValueSet(s.Resource))
	setIfNotNil(stmt, "NotResource", normalizeValueSet(s.NotResource))
	setIfNotNil(stmt, "Condition", normalizeCondition(s.Condition))

	b, err := json.Marshal(stmt)
	if err != nil {
		return "", errors.Wrapf(err, "failed to encode canonical policy statement %q", s.Sid)
	}
	return string(b), nil
}

func setIfNotNil(m map[string]any, key string, v any) {
	if v != nil {
		m[key] = v
	}
}

// normalizeValueSet converts a union-typed policy value (a bare string or a
// list of strings) into a sorted string slice so that a bare string equals
// the equivalent single-element array and list ordering is insignificant.
// Scalar values (bool/number) compare equal to their string forms, matching
// how IAM interprets them.
func normalizeValueSet(v any) any {
	switch v := util.FixMapKeys(v).(type) {
	case nil:
		return nil
	case string:
		return []string{v}
	case []string:
		out := slices.Clone(v)
		slices.Sort(out)
		return out
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = fmt.Sprint(item)
		}
		slices.Sort(out)
		return out
	default:
		return []string{fmt.Sprint(v)}
	}
}

// normalizePrincipal canonicalizes a Principal/NotPrincipal value. The value
// is either the wildcard string "*" or a map of principal type (AWS, Service,
// Federated, CanonicalUser) to one or more identifiers.
func normalizePrincipal(v any) any {
	switch v := util.FixMapKeys(v).(type) {
	case nil:
		return nil
	case string:
		return v
	case map[string]string:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[CanonicalPrincipalKey(k)] = normalizeValueSet(val)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[CanonicalPrincipalKey(k)] = normalizeValueSet(val)
		}
		return out
	default:
		return v
	}
}

// normalizeCondition canonicalizes a Condition block: a map of condition
// operator to a map of condition key to one or more values.
func normalizeCondition(v any) any {
	fixed := util.FixMapKeys(v)
	m, ok := fixed.(map[string]any)
	if !ok {
		return fixed
	}
	out := make(map[string]any, len(m))
	for op, keys := range m {
		km, ok := util.FixMapKeys(keys).(map[string]any)
		if !ok {
			out[op] = keys
			continue
		}
		nk := make(map[string]any, len(km))
		for key, vals := range km {
			nk[key] = normalizeValueSet(vals)
		}
		out[op] = nk
	}
	return out
}
