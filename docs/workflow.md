# Buildit Workflow

**buildit** takes the following steps after invocation.

### Step 1: Read Config
The first step is to read the yaml file supplied with the `--path` flag. If no value is supplied, or the file does not exist, **buildit** will return an error and exit. After reading the contents, all variables are replace by their corresponding values as supplied with command-line flag `--variables`. If the value of any variable is missing, **buildit** will stop processing & exit with error.

### Step 2: Read Overrides
Next, if an override yaml file is supplied with `--override` flag, it is read & all variables are substituted with the supplied values. As with config, if a variable value is not supplied, **buildit** will exit with an error. 

After reading the contents, all variables are replace by their corresponding values as supplied with command-line flag `--variables`. If the value of any variable is missing, **buildit** will stop processing & exit with error.

> For more details on the structure of the override file, and available features check out [Config Overrides](./overrides.md)


### Step 3: Initialize AWS Provider
Next, the default & other AWS providers are configured. All providers are checked to make sure they have valid credentials. If any of the providers fail the check, **buildit** will report an error & exit. 

**Note:** that the validation of providers only check configuration supplied to fetch credentials only; it does not check permissions supplied for the required operations. Those kind of errors are only found during exeuction.

### Step 4: Parse
Next ***buildit** generates a stream of bytes for each config files, after including any include/template files as per the rules of inclusion, and expanding/subsituting any variables for their supplied values; then parses the content into it's internal data structures, a directed acyclic graph (DAG). All resources defined in the **buildit** config are represented as vertices of the DAG. Any resource with a `dependsOn` list defined gets an edge from the resources it depends on, into itself. Any resources with no inward edges are independent.

If variable values are not supplied at runtime, template reference cannot be resolved, or resources have missing or circular dependencies, **buildit** will report an error an exit. 

### Step 5: Normalize
Once a valid DAG is built, all defined resources are normalized. Normalization includes setting default values for attributes that are not supplied, and also converting any values from `buildit` representation to internal AWS representation like constants or enumerated values etc.

### Step 6: Validation
After normalization, ***buildit** runs a semantic valiation. This step checks the input attributes supplied & reports any possible errors it could detect. This step may not find all possible issues, especially the ones that are only known at runtime. At this point, an AWS dry-run is not attempted. It however does a best effort validation to prevent running buggy configuration that may cause errors during execution.

### Step 7: Execution

A topologically sorted list of resources is generated from the DAG. **buildit** executes the`apply`, `destroy` or `plan` commands using this list. If the optional `--targets` flag is supplied, only the targetted resources are executed. 

For `apply` and `plan`, the DAG is processed using in-order topological list processing; starting with the independent nodes and towards the dependent nodes.

For `destroy`, the topological sort list is processed in reverse order, so the most dependant resource is destroyed before the independant ones.

> When using `--targets` you have to be careful to not violate the dependency ordering for either of the above commands, i.e targetting a dependant resource & not the independant resource when `apply`-ing.






