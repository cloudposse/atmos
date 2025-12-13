/**
 * Tests for YAML to JSON/HCL converter.
 */
const { convertYamlToFormats, parseYamlWithFunctions, generateJson, generateHcl } = require('./converter');

describe('convertYamlToFormats', () => {
  test('converts simple YAML to all formats', () => {
    const yaml = `settings:
  region: us-west-2
  timeout: 30`;

    const result = convertYamlToFormats(yaml);

    expect(result.yaml).toContain('region: us-west-2');
    expect(result.json).toContain('"region": "us-west-2"');
    expect(result.hcl).toContain('region = "us-west-2"');
  });

  test('converts nested objects', () => {
    const yaml = `database:
  host: localhost
  port: 5432
  credentials:
    username: admin`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('"host": "localhost"');
    expect(result.json).toContain('"port": 5432');
    expect(result.hcl).toContain('host = "localhost"');
    expect(result.hcl).toContain('port = 5432');
  });

  test('converts arrays', () => {
    const yaml = `tags:
  - production
  - critical`;

    const result = convertYamlToFormats(yaml);

    // JSON is pretty-printed.
    expect(result.json).toContain('"production"');
    expect(result.json).toContain('"critical"');
    expect(result.hcl).toContain('"production"');
    expect(result.hcl).toContain('"critical"');
  });

  test('converts YAML with !env function', () => {
    const yaml = `settings:
  region: !env AWS_REGION`;

    const result = convertYamlToFormats(yaml);

    expect(result.yaml).toContain('!env AWS_REGION');
    expect(result.json).toContain('${env:AWS_REGION}');
    expect(result.hcl).toContain('atmos::env("AWS_REGION")');
  });

  test('converts YAML with !exec function', () => {
    const yaml = `version: !exec "git describe --tags"`;

    const result = convertYamlToFormats(yaml);

    // The quotes are preserved in the argument parsing.
    expect(result.json).toContain('${exec:');
    expect(result.json).toContain('git describe --tags');
    expect(result.hcl).toContain('atmos::exec(');
    expect(result.hcl).toContain('git describe --tags');
  });

  test('converts YAML with !terraform.output function', () => {
    const yaml = `vpc_id: !terraform.output vpc.id stack=prod-us-west-2`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('${terraform.output:vpc.id stack=prod-us-west-2}');
    expect(result.hcl).toContain('atmos::terraform_output');
  });

  test('converts YAML with !store function', () => {
    const yaml = `api_key: !store ssm/api/key`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('${store:ssm/api/key}');
    expect(result.hcl).toContain('atmos::store("ssm", "api/key")');
  });

  test('preserves boolean values', () => {
    const yaml = `enabled: true
disabled: false`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('"enabled": true');
    expect(result.json).toContain('"disabled": false');
    expect(result.hcl).toContain('enabled = true');
    expect(result.hcl).toContain('disabled = false');
  });

  test('preserves null values', () => {
    const yaml = `optional: null`;

    const result = convertYamlToFormats(yaml);

    expect(result.json).toContain('"optional": null');
    expect(result.hcl).toContain('optional = null');
  });

  test('converts multi-document YAML with explicit stack names', () => {
    const yaml = `name: dev
vars:
  env: development
---
name: prod
vars:
  env: production`;

    const result = convertYamlToFormats(yaml);

    // JSON should be an array
    expect(result.json).toContain('[');

    // HCL should have multiple stack blocks with names
    expect(result.hcl).toContain('stack "dev" {');
    expect(result.hcl).toContain('stack "prod" {');
    expect(result.hcl).toContain('env = "development"');
    expect(result.hcl).toContain('env = "production"');
  });

  test('converts multi-document YAML without explicit names', () => {
    const yaml = `components:
  terraform:
    vpc: {}
---
components:
  terraform:
    eks: {}`;

    const result = convertYamlToFormats(yaml);

    // Should have stack blocks without labels
    expect(result.hcl).toMatch(/stack \{/);
    expect(result.hcl).toContain('component "vpc" {');
    expect(result.hcl).toContain('component "eks" {');
  });

  test('uses block syntax for nested objects in HCL', () => {
    const yaml = `settings:
  region: us-west-2
  nested:
    value: 42`;

    const result = convertYamlToFormats(yaml);

    // Block syntax: "key {" not "key = {"
    expect(result.hcl).toContain('settings {');
    expect(result.hcl).toContain('nested {');
    expect(result.hcl).toContain('region = "us-west-2"');
    expect(result.hcl).toContain('value = 42');
  });
});

describe('parseYamlWithFunctions', () => {
  test('parses standard YAML', () => {
    const yaml = `key: value`;
    const result = parseYamlWithFunctions(yaml);

    expect(result).toEqual({ key: 'value' });
  });

  test('parses YAML with !env tag', () => {
    const yaml = `region: !env AWS_REGION`;
    const result = parseYamlWithFunctions(yaml);

    expect(result.region.__atmosFunction).toBe('env');
    expect(result.region.__atmosArg).toBe('AWS_REGION');
  });

  test('parses YAML with !template tag', () => {
    const yaml = `config: !template "{{ .Values.name }}"`;
    const result = parseYamlWithFunctions(yaml);

    expect(result.config.__atmosFunction).toBe('template');
    // Quotes are preserved in the raw argument.
    expect(result.config.__atmosArg).toContain('{{ .Values.name }}');
  });
});

describe('generateJson', () => {
  test('generates formatted JSON', () => {
    const data = { key: 'value', nested: { inner: 123 } };
    const result = generateJson(data);

    expect(result).toBe(JSON.stringify(data, null, 2));
  });
});

describe('generateHcl', () => {
  test('generates HCL for simple object using block syntax', () => {
    const data = { region: 'us-west-2' };
    const result = generateHcl(data);

    expect(result).toContain('region = "us-west-2"');
  });

  test('generates HCL for nested objects using block syntax', () => {
    const data = {
      settings: {
        timeout: 30,
        enabled: true,
      },
    };
    const result = generateHcl(data);

    // Block syntax uses "settings {" not "settings = {"
    expect(result).toContain('settings {');
    expect(result).toContain('timeout = 30');
    expect(result).toContain('enabled = true');
  });

  test('generates stack block with name label for full stacks', () => {
    const data = {
      name: 'production',
      vars: {
        region: 'us-west-2',
      },
    };
    const result = generateHcl(data);

    expect(result).toContain('stack "production" {');
    expect(result).toContain('vars {');
    expect(result).toContain('region = "us-west-2"');
    expect(result).not.toContain('name ='); // Name becomes label, not attribute
  });

  test('generates stack block without label for unnamed stacks', () => {
    const data = {
      components: {
        terraform: {},
      },
    };
    const result = generateHcl(data);

    expect(result).toContain('stack {');
    expect(result).not.toContain('stack "');
  });

  test('generates labeled component blocks', () => {
    const data = {
      components: {
        terraform: {
          vpc: {
            vars: {
              cidr: '10.0.0.0/16',
            },
          },
          eks: {
            vars: {
              cluster_name: 'my-cluster',
            },
          },
        },
      },
    };
    const result = generateHcl(data);

    expect(result).toContain('components {');
    expect(result).toContain('terraform {');
    expect(result).toContain('component "vpc" {');
    expect(result).toContain('component "eks" {');
    expect(result).toContain('cidr = "10.0.0.0/16"');
    expect(result).toContain('cluster_name = "my-cluster"');
  });
});

describe('generateMultiDocHcl', () => {
  test('generates multiple stack blocks from multiple documents', () => {
    const documents = [
      { name: 'dev', vars: { env: 'development' } },
      { name: 'prod', vars: { env: 'production' } },
    ];

    const { generateMultiDocHcl } = require('./converter');
    const result = generateMultiDocHcl(documents);

    expect(result).toContain('stack "dev" {');
    expect(result).toContain('stack "prod" {');
    expect(result).toContain('env = "development"');
    expect(result).toContain('env = "production"');
  });
});
