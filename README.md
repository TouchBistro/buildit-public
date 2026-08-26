> **Note:** This is a public mirror of TouchBistro's internal `buildit` repository, published at [`github.com/TouchBistro/buildit-public`](https://github.com/TouchBistro/buildit-public). It is provided for community visibility and reuse, not as a live development repo. Pull requests cannot be merged; changes flow from the internal canonical repo, not into it. Code is provided as-is under the [MIT license](LICENSE).

---
**buildit** is a simple CLI-based infrastructure-as-code (IaC) tool developed at [Touchbistro](www.touchbistro.com). The main design objective behind building yet another IaC tool was to simplify the provisioning & updating of AWS resources that sit close to our application stack; for instance load-balancers, target groups, ECS task definition, services, EC2 Security Groups, & etc.

> For a complete list of supported resources & their configuration please refer to the [Supported Resources](./docs/resources/resources.md) section.

**buildit** uses a declerative approach to defining infrastructure. It provides a [YAML](https://yaml.org/)-based [DSL](https://en.wikipedia.org/wiki/Domain-specific_language) that is intuitive & closely resembles the AWS API payload structures. It also follows the 'WYSIWYG' approach as it only relies on what is defined in the YAML config & what already exists in AWS. It does not store or look for any persisted state information from previous runs. While this seems like a limitation, - _i.e **buildit** cannot detect & retain any manually introduced drifts in configuration, or resource deletions;_ it does however simplify using the tool to maintain infrastructure state to what is defined without the need of extensive planning and review of **buildit** run side-effects.

To view more details on how to use the **buildit** CLI see: [buildit CLI](./docs/cli.md)

For details on the structure & definition of resources in the yaml decleration check out: [buildit config](./docs/config.md).

To understand how buildit works under the hood, see: [buildit workflow](./docs/workflow.md)
