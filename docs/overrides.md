# Overrides

> IMPORTANT THIS DOCUMENTATION PAGE IS **DRAFT**; NEEDS REVIEW AND UPDATES


**buildit** provides override option that can be applied to select supported sections of the buildit YAML file to add or update certain resource definitions without having to modify the targetted buildit config yaml file.

We recommend supplying the override configuration using a command-line flag, so that it’s not magically applied, rather it is explicit. 



```
% buildit apply –-path ./buildit.yml \ 
                –-override ./override.yml \ 
                –-variables env=prod 
``````

## Provider & Resources

The following rules will be applied for the overrides. 

The schema of the overrides configuration will be the same as the regular buildit config. For each supported resource, any matching section will be considered for applying the overrides.

For the scope of this proposal, a resource means a supported section of buildit, that supports override features. 

The following sections support overrides

- providers  
- resources -> security-group


## Considerations

The following will be considered when applying an override:

- If the reserved * “override” resource name is used, then all the override attributes with a non-default value will be copied to all defined resources in that section. (a Merge operation)
- If the “override” resource name (can be a regex string) is used, then the overridden attributes will be applied to all  matching resources in that section.
If the “override” resource name does not match with any existing resources defined in that section, then a new resource (provider, security group etc) will be added (an Append operation) - If the override resource name was a regex, and it matches no resources, then a new resource will only be added if the regex is a valid resource name. During the override process only the following characters are allowed when creating new resources: a-z, A-Z, 0-9, - (dash) and _ (underscore)
- Any default /implicit definitions, e.g the `main` provider when not defined explicitly in the buildit yaml cannot be updated, although can be added. 
- Parsing & applying overrides will happen during the Parse config phase of “buildit”. The Validation phase will run after the overrides have been applied & the resource may fail validation if applying the override causes the resource configuration to become invalid.
- When writing override configuration, the end user must be careful & try to visualize the effects of the overrides when applied to a buildit config. If you expect an override to always merge with existing config, partial definition of resource attributes will work. However, if an override 

## Examples


### Example 1: 
The following override adds or update an existing provider matching the named `it`:

```yaml
providers: 
  it:
    type: role
    accountId: 123456789012
    roleName: Buildit-Role
```

### Example 2:
The following override would update the `roleName` attribute for all providers defined.
```yaml
providers: 
  ‘*’:
    roleName: Buildit-Role
 ```

### Example 3:
The following adds a new inbound rule (if a matching rule doesn’t exist already; else it will create a new securityGroup reference  under the matching rule (same portRange & ipProtocol) since the matcher is used, this will apply a merge config to all existing security group definitions in ***buildit***

```yaml
resources: 
  security-group:
    ‘*’ :
      inboundRules: 
         portRange: 8080
         ipProtocol: tcp 
         securityGroups: 
         - value: it/vpn-sg 
           description: allow from backup VPN
```


### Example 4:
The following will find all security-group definitions that matches the regex `^.*svc-sg.*$` & adds a new inbound rule if a matching rule doesn’t exist already; else it will create a new securityGroup reference under the matching rule (same `portRange` & `ipProtocol`) 


```yaml
    ‘^.*svc-sg.*$’ :
      inboundRules: 
         portRange: 8080
         ipProtocol: tcp 
         securityGroups: 
         - value: it/vpn-sg 
           description: allow from backup VPN
```
  
### Example 5
Tthe following checks if a security-group definition with the name `test-sg` already exists; if it does, then a new inboundRule is added; otherwise a new security group definition is added. In cases where override results in a new `security-group` resource being provisioned, the validations will be applied and fail it not all fields are supplied.

```yaml
  security-group:
    test-sg :
      inboundRules: 
         portRange: 8080
         ipProtocol: tcp 
         securityGroups: 
         - value: it/vpn-sg 
           description: allow from VPN
```

