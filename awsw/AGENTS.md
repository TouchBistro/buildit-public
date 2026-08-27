# AWS Wrapper Package `awsw`

This package provides high-level, context-aware wrappers around the AWS SDK v2 clients.

## Key Responsibilities

- Encapsulate AWS SDK interactions to provide a cleaner API for resources.
- Handle common AWS operations like tag management, ARN resolution from names, and pagination.
- Provide consistent error handling and logging for AWS API calls.

## Standard Patterns

### Client Creation

Each service has a wrapper struct and a constructor:

```go
type ServiceName struct {
    *service.Client
}

func NewServiceName(ctx context.Context, providerName string) ServiceName {
    return ServiceName{client.ServiceName(ctx, providerName)}
}
```

### Client Access

- Use client package functions: `client.ACM()`, `client.SNS()`, etc.
- Clients are context-aware and provider-aware:
  ```go
  acmClient := client.ACM(ctx, c.Context.ProviderName)
  ```

### ARN Resolution (`[Resource]ArnForIdentifier`)

Resources often need to resolve an ARN from a user-supplied identifier (ARN, ID, or Name). These methods follow a standardized tiered lookup pattern:

- **Input**: `identifier string` (may include provider prefix, e.g., `staging::my-resource`).
- **Standardized Parsing**: Use `awsw.ParseIdentifier(identifier)` to extract the resource and provider.
- **Lookup Strategy**:
  1. **Tier 0 (Direct)**: If the identifier is a valid ARN, return it immediately after verification (optional).
  2. **Tier 1 (ID)**: If it looks like a physical ID (e.g., `vpc-xxx`, `subnet-xxx`), perform a direct `Describe` or `Get` call.
  3. **Tier 2 (Name/Tag)**: Fallback to listing or filtering by Name (e.g., using `tag:Name` for EC2/ECS or direct name lookups for S3/SNS).
- **Return**: `(*string, error)`. Return `nil, nil` if the resource is explicitly missing where appropriate, or a descriptive error if resolution fails.
- **Ambiguity**: If multiple matches are found for a name/tag, return an error.

## Guidelines

- **Nil-check SDK response pointers before dereferencing.** AWS SDK v2 output structs expose
  nested data as pointers (`out.FunctionSummary.FunctionMetadata`, `out.VpcOrigin.Status`, …) and
  a partial/malformed response makes any unguarded chain panic mid-apply with a stack trace
  instead of a wrapped error. Guard every level you dereference and return a descriptive error:
  ```go
  if out.FunctionSummary == nil || out.FunctionSummary.FunctionMetadata == nil {
      return errors.Errorf("cloudfront function %q has an incomplete response", name)
  }
  ```
  `aws.ToString`/`aws.ToBool` are nil-safe for leaf fields, but they do not protect the struct
  pointers on the way to the leaf.
- **No Resource Logic**: These wrappers should be purely for interacting with AWS. Business logic belongs in the `resource` package.
- **Context Propagation**: Always pass `context.Context` as the first parameter to SDK calls and helper methods.
- **Error Wrapping**: Always use `github.com/pkg/errors` — not `fmt.Errorf` — for wrapping errors. Use `errors.Wrapf(err, "...")` to add context and `errors.Errorf("...")` for new errors. Do **not** use `fmt.Errorf` with `%w`.
- **Retry and Backoff**: Use context values for retry/backoff configuration. Implement exponential backoff for transient failures.
- **Logging**: Use `logrus` for debugging API interactions. Use `log.Tracef` or `log.Debugf` for verbose output.
