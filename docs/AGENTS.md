# Documentation Standards

This document outlines the standards for writing resource documentation in `docs/resources`.

## File Structure

Each resource documentation file should follow this structure:

1.  **Title**: `# Resource Name `code-name``
2.  **Description**: A brief description of what the resource does (Create, update, or destroy...).
3.  **Provider Note**: A standard blockquote about provider prefixes.
4.  **AWS Documentation Link**: A link to the relevant AWS CLI documentation.
5.  **Main Resource Table**: A markdown table listing the top-level fields.
6.  **Example**: A YAML example showing how to configure the resource.
7.  **Sub-structure Tables**: Separate sections and tables for complex nested structures (e.g., `Container`, `LogConfiguration`).

## Table Format

All tables should have the following columns:

| Field       | Description              | DataType | Required | Default         |
| :---------- | :----------------------- | :------- | :------- | :-------------- |
| `fieldName` | Description of the field | `type`   | Yes/No   | `default_value` |

- **Field**: The YAML key name (e.g., `name`, `memory`).
- **Description**: What the field controls.
- **DataType**: The type (e.g., `string`, `int32`, `bool`, `[]string`, `map[string]string`, or a custom struct name like `LogConfiguration`).
- **Required**: "Yes" or "No".
- **Default**: The default value if applicable, wrapped in backticks.

## Standard Text Blocks

### Provider Note

```markdown
> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.
```

### AWS Link

Format: `Check out AWS documentation for [resource name] [here](url).`

## Example Section

Use a `yaml` code block to provide a comprehensive example.

## Sub-structures

For fields that map to a complex struct (like `containers` in a task definition), create a new Level 2 Header (`## StructName`) below the example, followed by a description and its own table.
