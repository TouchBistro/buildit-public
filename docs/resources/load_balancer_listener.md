# Load Balancer Listener `listeners` 

Child resource of [load-balancer](./load_balancer.md).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

This is a child resource of [load balancer](./load_balancer.md). Creates or updates a listener and associates it with the specified load balancer. Check out AWS documentation for load balancer listener [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-listener.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`name`|A name for the load balancer listener |`string`|Yes||
|`certificates`|The list of certificates for the listener. First one is considered default |`[]string`|Yes||
|`protocol`|The protocol for connections from clients to the load balancer. For `application` type, valid values: `HTTP` and `HTTPS`. For `network` type, valid values: `TCP`, `TLS`, `UDP` |`string`|Yes||
|`sslPolicy`|The security policy that defines which protocols and ciphers are supported |`string`|No|`ELBSecurityPolicy-TLS13-1-1-2021-06`|
|`port`|The port on which the load balancer is listening |`int32`|Yes||
|`ifNoRulesMatch`|Default rule if no other rule is matched. See [LBRule](#lbrule) section for more details |`LBRule`|Yes||
|`rules`|A list of rules for the listener. See [LBRule](#lbrule) section for more details |`[]LBRule`|No|`[]`|

## LBRule
Creates or updates a load balancer rule and associates it with the specified listener. Check out AWS documentation for load balancer rule [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-rule.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`if`|A list of rules for the listener. See [LBCondition](#lbcondition) section for more details |`[]LBCondition`|No|`[]`|
|`then`|A list of rules for the listener. See [LBAction](#lbaction) section for more details |`[]LBAction`|No|`[]`|

## LBCondition
Creates or updates a load balancer rule and associates it with the specified listener. Check out AWS documentation for load balancer rule [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-rule.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`the`|Field in the HTTP request. Valid values: `host-header`, `path-pattern`, `http-header` |`string`|Yes||
|`named`|The name of the HTTP header field. The maximum size is 40 characters. The header name is case insensitive. Wildcards not supported |`string`|Yes||
|`is`|The list of strings to compare against the value of the HTTP header. The maximum size of each string is 128 characters. The comparison strings are case insensitive |`[]string`|Yes||

## LBAction
Creates or updates a load balancer rule and associates it with the specified listener. Check out AWS documentation for load balancer rule [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-rule.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`do`|Type of action. Valid values: `redirect`, `forward`, `redirect-https`, `fixed-response`, `authenticate-oidc` |`string`|Yes||
|`fixedContentType`|Only `fixed-response` type. The content type of the custom HTTP response |`string`|No||
|`fixedMessageBody`|Only `fixed-response` type. The message body of the custom HTTP response |`string`|No||
|`fixedStatusCode`|Only `fixed-response` type. The status code of the custom HTTP response |`string`|No||
|`forwardStickinessDuration`|Only `forward` type. The period, in seconds, during which requests from a client should be routed to the same target group. If no value is given, stickiness is disabled |`int32`|No||
|`forwardTargetGroups`|Only `forward` type. The target groups to forward the request to. `LBTargetGroupRef` is a struct with two fields `name` and `weight`|`[]LBTargetGroupRef`|No||
|`redirectHost`|Only `redirect` type. The host to redirect requests to|`string`|No||
|`redirectPath`|Only `redirect` type. The path to redirect requests to|`string`|No||
|`redirectPort`|Only `redirect` type. The port to redirect requests to|`string`|No||
|`redirectProtocol`|Only `redirect` type. The protocol for redirect. Valid values: `HTTP`, `HTTPS`, or `#{protocol}` |`string`|No||
|`redirectQuery`|Only `redirect` type. The query parameteres for redirect, URL-encoded when necessary, but not percent-encoded. The leading `?` is automatically added |`string`|No||
|`redirectStatusCode`|Only `redirect` type. The HTTP redirect code. Valid values: `HTTP_301`, `HTTP_302`|`string`|No||
|`authenticateOidcConfig`|Only `authenticate-oidc` type. Information about an identity provider that is compliant with OpenID Connect. Check [Authenticate Oidc Config](#authenticateoidcconfig) |`AuthenticateOidcConfig`|No||

## AuthenticateOidcConfig

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`issuer`|The OIDC issuer identifier of the IdP. This must be a full URL, including the HTTPS protocol, the domain, and the path |`string`|Yes||
|`authorizationEndpoint`|The authorization endpoint of the IdP. This must be a full URL, including the HTTPS protocol, the domain, and the path |`string`|Yes||
|`tokenEndpoint`|The token endpoint of the IdP. This must be a full URL, including the HTTPS protocol, the domain, and the path |`string`|Yes||
|`userInfoEndpoint`|The user info endpoint of the IdP. This must be a full URL, including the HTTPS protocol, the domain, and the path |`string`|Yes||
|`clientId`|The OAuth 2.0 client identifier |`string`|Yes||
|`clientSecret`|The OAuth 2.0 client secret |`string`|Yes||
|`sessionCookie`|The name of the cookie used to maintain session information |`string`|No|`AWSELBAuthSessionCookie`|
|`sessionTimeoutSeconds`|The maximum duration of the authentication session, in seconds |`int64`|No|`604800`|
|`scope`|The set of user claims to be requested from the IdP |`string`|No|`openid`|
|`params`|The query parameters (up to 10) to include in the redirect request to the authorization endpoint |`map[string]string`|No|`{}`|
|`onUnauthenticatedRequest`|The behavior if the user is not authenticated. Valid values: `deny`, `allow`, `authenticate` |`string`|No|`authenticate`|

Example:

```yaml 

resources:
  load-balancer:
    example-web-lb:
    ...
      listeners:
        - name: http:80
          protocol: HTTP
          port: 80
          ifNoRulesMatch:
            then:
              - do: redirect-https
        - name: http:443
          protocol: HTTPS
          port: 443
          certificates:
            - my.example.com
          ifNoRulesMatch:
            then:
              - do: fixed-response
                fixedContentType: "text/plain"
                fixedMessageBody: "Target not found"
                fixedStatusCode: 404
          rules:
            - if:
                - the: host-header
                  is:
                    - my.example.com
                - the: path-pattern
                  is:
                    - /*
              then:
                - do: forward
                  forwardStickiness: 0
                  forwardTargetGroups:
                    - name: example-web-waf-tg
                      weight: 100
```